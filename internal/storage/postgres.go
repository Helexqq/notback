package storage

import (
	"contest/internal/config"
	"context"
	"fmt"

	"time"

	"contest/pkg/util"

	"github.com/jackc/pgx/v4/pgxpool"
)

const (
	createTablesQuery = `
	CREATE TABLE IF NOT EXISTS sales (
		id TEXT PRIMARY KEY,
		start_time TIMESTAMPTZ NOT NULL,
		total_items INT NOT NULL,
		items_sold INT DEFAULT 0
	);
	
	CREATE TABLE IF NOT EXISTS checkouts (
		id SERIAL PRIMARY KEY,
		sale_id TEXT REFERENCES sales(id),
		user_id TEXT NOT NULL,
		item_id TEXT NOT NULL,
		item_name TEXT NOT NULL DEFAULT '',
		item_image_url TEXT NOT NULL DEFAULT '',
		code TEXT NOT NULL UNIQUE,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	
	CREATE TABLE IF NOT EXISTS purchases (
		id SERIAL PRIMARY KEY,
		checkout_id INT REFERENCES checkouts(id),
		purchased_at TIMESTAMPTZ DEFAULT NOW()
	);
	
	CREATE INDEX IF NOT EXISTS idx_checkouts_code ON checkouts(code);
	`
)

type PostgresPool struct {
	*pgxpool.Pool
}

func NewPostgresPool(ctx context.Context, cfg config.PostgresConfig) (*PostgresPool, error) {
	pool, err := pgxpool.Connect(ctx, cfg.GetPostgresURL())
	if err != nil {
		return nil, util.Wrap(err)
	}
	return &PostgresPool{pool}, nil
}

func (p *PostgresPool) Close() {
	p.Pool.Close()
}

func MigratePostgres(ctx context.Context, pool *PostgresPool) error {
	_, err := pool.Exec(ctx, createTablesQuery)
	return util.Wrap(err)
}

func RecordNewSale(ctx context.Context, pool *PostgresPool, saleID string, totalItems int) error {
	_, err := pool.Exec(ctx, `
		INSERT INTO sales (id, start_time, total_items) 
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET start_time = EXCLUDED.start_time,
			total_items = EXCLUDED.total_items,
			items_sold = 0
	`, saleID, time.Now().UTC(), totalItems)
	return util.Wrap(err, saleID, totalItems)
}

func RecordCheckout(ctx context.Context, pool *PostgresPool, saleID, userID, itemID, code string, itemName, itemImageURL string) (int, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM checkouts 
			WHERE sale_id = $1 AND item_id = $2
		)
	`, saleID, itemID).Scan(&exists)
	if err != nil {
		return 0, fmt.Errorf("failed to check item availability")
	}
	if exists {
		return 0, fmt.Errorf("item already sold")
	}

	var userCount int
	err = tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM checkouts 
		WHERE sale_id = $1 AND user_id = $2
	`, saleID, userID).Scan(&userCount)
	if err != nil {
		return 0, fmt.Errorf("failed to check user limit")
	}
	if userCount >= 10 {
		return 0, fmt.Errorf("user limit reached")
	}

	var totalSold int
	err = tx.QueryRow(ctx, `
		SELECT items_sold FROM sales 
		WHERE id = $1
	`, saleID).Scan(&totalSold)
	if err != nil {
		return 0, fmt.Errorf("failed to check total sold")
	}
	if totalSold >= 10000 {
		return 0, fmt.Errorf("sale sold out")
	}

	var checkoutID int
	err = tx.QueryRow(ctx, `
		INSERT INTO checkouts (sale_id, user_id, item_id, item_name, item_image_url, code)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`, saleID, userID, itemID, itemName, itemImageURL, code).Scan(&checkoutID)
	if err != nil {
		return 0, fmt.Errorf("failed to insert checkout")
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction")
	}

	return checkoutID, nil
}

func RecordPurchase(ctx context.Context, pool *PostgresPool, saleID, userID, itemID, code string, checkoutID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM checkouts 
			WHERE id = $1 AND code = $2
		)
	`, checkoutID, code).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check checkout existence")
	}
	if !exists {
		return fmt.Errorf("checkout not found or code mismatch")
	}

	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM purchases 
			WHERE checkout_id = $1
		)
	`, checkoutID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check purchase existence")
	}
	if exists {
		return fmt.Errorf("purchase already exists")
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO purchases (checkout_id)
		VALUES ($1)
	`, checkoutID)
	if err != nil {
		return fmt.Errorf("failed to insert purchase")
	}

	_, err = tx.Exec(ctx, `
		UPDATE sales 
		SET items_sold = items_sold + 1
		WHERE id = $1
	`, saleID)
	if err != nil {
		return fmt.Errorf("failed to update items sold count")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction")
	}

	return nil
}

func RollbackCheckout(ctx context.Context, pool *PostgresPool, checkoutID int) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction")
	}
	defer tx.Rollback(ctx)

	var saleID string
	var itemID string
	err = tx.QueryRow(ctx, `
		SELECT sale_id, item_id FROM checkouts 
		WHERE id = $1
	`, checkoutID).Scan(&saleID, &itemID)
	if err != nil {
		return fmt.Errorf("failed to get checkout info")
	}

	_, err = tx.Exec(ctx, `
		DELETE FROM checkouts 
		WHERE id = $1
	`, checkoutID)
	if err != nil {
		return fmt.Errorf("failed to delete checkout")
	}

	_, err = tx.Exec(ctx, `
		UPDATE sales 
		SET items_sold = items_sold - 1
		WHERE id = $1
	`, saleID)
	if err != nil {
		return fmt.Errorf("failed to update items sold count")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction")
	}

	return nil
}
