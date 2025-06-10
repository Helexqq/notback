package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"contest/internal/storage"
	"contest/pkg/util"

	"github.com/redis/go-redis/v9"
)

const (
	itemsPerSale    = 10000
	maxItemsPerUser = 10
	reservationTTL  = 5 * time.Minute
	saleDuration    = time.Hour
	checkoutScript  = `
		local item_id = ARGV[1]
		local user_id = ARGV[2]
		local reservation_code = ARGV[3]
		local sale_id = ARGV[4]

		if redis.call('EXISTS', 'used_code:' .. reservation_code) == 1 then
			return 'CODE_USED'
		end

		local available_key = 'sale:' .. sale_id .. ':available'
		if redis.call('SISMEMBER', available_key, item_id) == 0 then
			return 'ITEM_SOLD_OUT'
		end

		local details_key = 'sale:' .. sale_id .. ':details'
		if redis.call('HEXISTS', details_key, item_id) == 0 then
			return 'ITEM_SOLD_OUT'
		end

		local user_key = 'sale:' .. sale_id .. ':user:' .. user_id
		local user_count = redis.call('GET', user_key) or 0
		if tonumber(user_count) >= 10 then
			return 'USER_LIMIT_REACHED'
		end

		local total_sold = redis.call('GET', 'sale:' .. sale_id .. ':total_sold') or 0
		if tonumber(total_sold) >= 10000 then
			return 'SALE_SOLD_OUT'
		end

		redis.call('SREM', available_key, item_id)
		redis.call('INCR', user_key)
		redis.call('INCR', 'sale:' .. sale_id .. ':total_sold')
		redis.call('SET', 'reservation:' .. reservation_code, sale_id .. ':' .. user_id .. ':' .. item_id .. ':' .. reservation_code)
		redis.call('SET', 'used_code:' .. reservation_code, '1', 'EX', 3600) 

		return 'OK'
	`
)

var (
	ErrItemSoldOut            = errors.New("item sold out")
	ErrUserLimitReached       = errors.New("user limit reached")
	ErrInvalidCode            = errors.New("invalid reservation code")
	ErrCodeUsed               = errors.New("reservation code already used")
	ErrSaleNotActive          = errors.New("no active sale")
	ErrSaleSoldOut            = errors.New("sale has reached total items limit")
	ErrInvalidReservationCode = errors.New("invalid reservation format")
)

type SaleService struct {
	redis          *storage.RedisClient
	pgPool         *storage.PostgresPool
	currentSale    string
	saleMutex      sync.RWMutex
	checkoutScript string
	localCache     *sync.Map
	recordChan     chan recordRequest
}

func NewSaleService(redis *storage.RedisClient, pgPool *storage.PostgresPool) *SaleService {
	svc := &SaleService{
		redis:          redis,
		pgPool:         pgPool,
		checkoutScript: checkoutScript,
		localCache:     &sync.Map{},
		recordChan:     make(chan recordRequest, itemsPerSale),
	}

	for i := 0; i < 10; i++ {
		go svc.recordWorker()
	}
	return svc
}

func (s *SaleService) recordWorker() {
	for req := range s.recordChan {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var err error
		switch req.operation {
		case "purchase":
			err = storage.RecordPurchase(ctx, s.pgPool, req.saleID, req.userID, req.itemID, req.reservationCode, req.checkoutID)
		}

		if err != nil {
			log.Printf("Failed to record %s: %v", req.operation, err)

		}
	}
}

func (s *SaleService) StartBackgroundTasks(ctx context.Context) {
	if err := s.StartNewSale(ctx); err != nil {
		log.Printf("Failed to start initial sale: %v", err)
	}
	ticker := time.NewTicker(saleDuration)
	defer ticker.Stop()

	for {
		select {

		case <-ctx.Done():
			return

		case <-ticker.C:
			if err := s.StartNewSale(ctx); err != nil {
				log.Printf("Failed to start new sale: %v", err)
			}
		}
	}
}

func (s *SaleService) StartNewSale(ctx context.Context) error {
	s.saleMutex.Lock()
	defer s.saleMutex.Unlock()

	saleID := time.Now().UTC().Format("2006010215")
	if err := s.resetRedisState(ctx, saleID); err != nil {
		return util.Wrap(err, saleID)
	}
	if err := storage.RecordNewSale(ctx, s.pgPool, saleID, itemsPerSale); err != nil {
		return util.Wrap(err, saleID)
	}

	s.currentSale = saleID

	log.Printf("Started new sale: %s", saleID)
	return nil
}

func (s *SaleService) resetRedisState(ctx context.Context, saleID string) error {
	items := make([]interface{}, itemsPerSale)
	itemDetailsMap := make(map[string]string)
	for i := 0; i < itemsPerSale; i++ {
		itemID := fmt.Sprintf("item_%d", i+1)

		// https://i.ibb.co/HDKpKRGp/open-me.jpg
		itemName := fmt.Sprintf("Durov Item #%d", i+1)
		imageURL := fmt.Sprintf("https://picsum.photos/seed/item_%d/200/300", i+1)
		// https://i.ibb.co/HDKpKRGp/open-me.jpg

		items[i] = itemID

		itemDetails := struct {
			Name     string `json:"name"`
			ImageURL string `json:"image_url"`
		}{
			Name:     itemName,
			ImageURL: imageURL,
		}
		jsonDetails, err := json.Marshal(itemDetails)
		if err != nil {
			return util.Wrap(err, "failed to marshal item details")
		}
		itemDetailsMap[itemID] = string(jsonDetails)
	}

	pipe := s.redis.Pipeline()
	availableKey := fmt.Sprintf("sale:%s:available", saleID)
	soldKey := fmt.Sprintf("sale:%s:sold", saleID)
	itemDetailsKey := fmt.Sprintf("sale:%s:details", saleID)

	pipe.SAdd(ctx, availableKey, items...)
	pipe.Set(ctx, soldKey, 0, 0)
	for itemID, details := range itemDetailsMap {
		pipe.HSet(ctx, itemDetailsKey, itemID, details)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return util.Wrap(err, "failed to prepare new sale state")
	}

	return nil
}

func (s *SaleService) CheckoutItem(ctx context.Context, userID, itemID string) (string, error) {
	s.saleMutex.RLock()
	currentSale := s.currentSale
	s.saleMutex.RUnlock()

	if currentSale == "" {
		return "", ErrSaleNotActive
	}

	itemDetailsKey := fmt.Sprintf("sale:%s:details", currentSale)
	itemDetailsJSON, err := s.redis.HGet(ctx, itemDetailsKey, itemID).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("item not found")
	}
	if err != nil {
		log.Printf("Error getting item details from Redis for item %s: %v", itemID, err)
		return "", fmt.Errorf("item not found")
	}

	var itemDetails struct {
		Name     string `json:"name"`
		ImageURL string `json:"image_url"`
	}
	if err := json.Unmarshal([]byte(itemDetailsJSON), &itemDetails); err != nil {
		log.Printf("Error unmarshaling item details for item %s: %v", itemID, err)
		return "", fmt.Errorf("item not found")
	}

	reservationCode, err := util.Reservation_сode() // Generate reservation code)
	if err != nil {
		return "", util.Wrap(err, "failed to generate reservation code")
	}

	checkoutID, err := storage.RecordCheckout(ctx, s.pgPool, currentSale, userID, itemID, reservationCode, itemDetails.Name, itemDetails.ImageURL)
	if err != nil {
		log.Printf("Failed to record checkout in PostgreSQL: %v (user: %s, item: %s)", err, userID, itemID)
		return "", err
	}

	args := []interface{}{
		itemID,
		userID,
		reservationCode,
		currentSale,
	}

	result, err := s.redis.Eval(ctx, s.checkoutScript, nil, args...).Result()
	if err != nil {
		if rbErr := storage.RollbackCheckout(ctx, s.pgPool, checkoutID); rbErr != nil {
			log.Printf("Failed to rollback checkout in PostgreSQL: %v (user: %s, item: %s)", rbErr, userID, itemID)
		}
		log.Printf("Error executing checkout script: %v (user: %s, item: %s)", err, userID, itemID)
		return "", util.Wrap(err, userID, itemID)
	}

	switch result {
	case "OK":
		s.redis.Set(ctx, fmt.Sprintf("checkoutid:%s", reservationCode), checkoutID, time.Hour)
		return reservationCode, nil

	case "CODE_USED":
		if rbErr := storage.RollbackCheckout(ctx, s.pgPool, checkoutID); rbErr != nil {
			log.Printf("Failed to rollback checkout in PostgreSQL: %v (user: %s, item: %s)", rbErr, userID, itemID)
		}
		log.Printf("Reservation code %s was already used (user: %s, item: %s)", reservationCode, userID, itemID)
		return "", ErrCodeUsed

	case "ITEM_SOLD_OUT":
		if rbErr := storage.RollbackCheckout(ctx, s.pgPool, checkoutID); rbErr != nil {
			log.Printf("Failed to rollback checkout in PostgreSQL: %v (user: %s, item: %s)", rbErr, userID, itemID)
		}
		log.Printf("Item %s is not available in sale %s (user: %s)", itemID, currentSale, userID)
		return "", ErrItemSoldOut

	case "USER_LIMIT_REACHED":
		if rbErr := storage.RollbackCheckout(ctx, s.pgPool, checkoutID); rbErr != nil {
			log.Printf("Failed to rollback checkout in PostgreSQL: %v (user: %s, item: %s)", rbErr, userID, itemID)
		}
		log.Printf("User %s has reached purchase limit in sale %s", userID, currentSale)
		return "", ErrUserLimitReached

	case "SALE_SOLD_OUT":
		if rbErr := storage.RollbackCheckout(ctx, s.pgPool, checkoutID); rbErr != nil {
			log.Printf("Failed to rollback checkout in PostgreSQL: %v (user: %s, item: %s)", rbErr, userID, itemID)
		}
		log.Printf("Sale %s has reached total items limit", currentSale)
		return "", ErrSaleSoldOut

	default:
		if rbErr := storage.RollbackCheckout(ctx, s.pgPool, checkoutID); rbErr != nil {
			log.Printf("Failed to rollback checkout in PostgreSQL: %v (user: %s, item: %s)", rbErr, userID, itemID)
		}
		log.Printf("Unexpected result from checkout script: %v (user: %s, item: %s)", result, userID, itemID)
		return "", fmt.Errorf("unexpected result: %v", result)
	}
}

type recordRequest struct {
	saleID          string
	userID          string
	itemID          string
	itemName        string
	itemImageURL    string
	reservationCode string
	checkoutID      int
	operation       string
}

func (s *SaleService) PurchaseItem(ctx context.Context, code string) error {
	reservationValue, err := s.redis.Get(ctx, fmt.Sprintf("reservation:%s", code)).Result()
	if err == redis.Nil {
		log.Printf("Reservation code %s not found in Redis", code)
		return ErrInvalidCode // Code not found
	} else if err != nil {
		log.Printf("Error getting reservation from Redis: %v", err)
		return util.Wrap(err, code)
	}
	deleted, err := s.redis.Del(ctx, fmt.Sprintf("reservation:%s", code)).Result()
	if err != nil {
		log.Printf("Error deleting reservation key %s: %v", code, err)
		return util.Wrap(err, code)
	}
	if deleted == 0 {
		log.Printf("Reservation code %s was already used or not found during delete attempt", code)
		return ErrCodeUsed
	}
	reservationParts := strings.Split(reservationValue, ":")
	if len(reservationParts) != 4 {
		log.Printf("Invalid reservation format for code %s, reservation value: %s, expected 4 parts, got %d", code, reservationValue, len(reservationParts))
		return ErrInvalidReservationCode
	}

	saleID := reservationParts[0]
	userID := reservationParts[1]
	itemID := reservationParts[2]

	log.Printf("Parsed reservation - SaleID: %s, UserID: %s, ItemID: %s", saleID, userID, itemID)

	checkoutIDStr, err := s.redis.Get(ctx, fmt.Sprintf("checkoutid:%s", code)).Result()
	if err != nil {
		log.Printf("Failed to get checkoutID for code %s: %v", code, err)
		return ErrInvalidCode
	}
	checkoutID, err := strconv.Atoi(checkoutIDStr)
	if err != nil {
		log.Printf("Invalid checkoutID for code %s: %v", code, err)
		return ErrInvalidCode
	}

	select {
	case s.recordChan <- recordRequest{
		saleID:          saleID,
		userID:          userID,
		itemID:          itemID,
		itemName:        "",
		itemImageURL:    "",
		reservationCode: code,
		checkoutID:      checkoutID,
		operation:       "purchase",
	}:
		log.Printf("Purchase record sent for code %s", code)
	default:
		log.Printf("Record channel full, dropping purchase record for %s", code)
	}

	return nil
}

func (s *SaleService) GetCurrentSale() string {
	s.saleMutex.RLock()
	defer s.saleMutex.RUnlock()
	return s.currentSale
}
