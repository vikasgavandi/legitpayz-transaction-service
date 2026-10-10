// Transaction Service: purchases, payments, refunds. Owns used_limit updates.
// Run: PORT=8003 go run ./services/transaction-service
package main

import (
	"os"

	"github.com/vikasgavandi/legitpayz-transaction-service/pkg/cache"
	"github.com/vikasgavandi/legitpayz-transaction-service/pkg/db"
	"github.com/vikasgavandi/legitpayz-transaction-service/pkg/events"
	"github.com/vikasgavandi/legitpayz-transaction-service/pkg/obs"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var rdb = cache.Client()

func main() {
	database, err := db.Connect()
	if err != nil {
		panic(err)
	}
	defer database.Close()

	r := obs.Engine("txn")

	// Purchase: authorise against limit, debit used_limit
	r.POST("/txns/purchase", func(c *gin.Context) {
		var req struct {
			CardID   string `json:"card_id" binding:"required"`
			Amount   int64  `json:"amount" binding:"required,gt=0"` // paise
			Merchant string `json:"merchant"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		var limit, used int64
		var status string
		if err := database.QueryRow(`SELECT credit_limit,used_limit,status FROM credit_cards WHERE id=$1`, req.CardID).Scan(&limit, &used, &status); err != nil {
			c.JSON(404, gin.H{"error": "card not found"})
			return
		}
		if status != "ACTIVE" {
			c.JSON(403, gin.H{"error": "card " + status})
			return
		}
		if used+req.Amount > limit {
			c.JSON(402, gin.H{"error": "insufficient limit", "available": limit - used})
			return
		}
		id := uuid.New().String()
		tx, _ := database.Begin()
		_, _ = tx.Exec(`INSERT INTO transactions(id,card_id,txn_type,amount,merchant) VALUES($1,$2,'PURCHASE',$3,$4)`, id, req.CardID, req.Amount, req.Merchant)
		_, _ = tx.Exec(`UPDATE credit_cards SET used_limit=used_limit+$1 WHERE id=$2`, req.Amount, req.CardID)
		_ = tx.Commit()
		cache.SetBalance(rdb, req.CardID, used+req.Amount)
		_ = events.Default.Publish("txn.events", req.CardID, `{"event":"PURCHASE","amount":`+itoa(req.Amount)+`}`)
		c.JSON(201, gin.H{"txn_id": id, "available": limit - used - req.Amount})
	})

	// Payment: reduce used_limit (bill payment)
	r.POST("/txns/payment", func(c *gin.Context) {
		var req struct {
			CardID string `json:"card_id" binding:"required"`
			Amount int64  `json:"amount" binding:"required,gt=0"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		id := uuid.New().String()
		tx, _ := database.Begin()
		_, _ = tx.Exec(`INSERT INTO transactions(id,card_id,txn_type,amount) VALUES($1,$2,'PAYMENT',$3)`, id, req.CardID, req.Amount)
		_, _ = tx.Exec(`UPDATE credit_cards SET used_limit=GREATEST(0,used_limit-$1) WHERE id=$2`, req.Amount, req.CardID)
		_ = tx.Commit()
		var used int64
		_ = database.QueryRow(`SELECT used_limit FROM credit_cards WHERE id=$1`, req.CardID).Scan(&used)
		cache.SetBalance(rdb, req.CardID, used)
		_ = events.Default.Publish("txn.events", req.CardID, `{"event":"PAYMENT"}`)
		c.JSON(201, gin.H{"txn_id": id, "used_limit": used})
	})

	r.GET("/txns/card/:id", func(c *gin.Context) {
		rows, _ := database.Query(`SELECT id,txn_type,amount,merchant,status,created_at FROM transactions WHERE card_id=$1 ORDER BY created_at DESC LIMIT 50`, c.Param("id"))
		defer rows.Close()
		var out []map[string]any
		for rows.Next() {
			var id, typ, merch, st, ts string
			var amt int64
			var m *string
			_ = rows.Scan(&id, &typ, &amt, &m, &st, &ts)
			if m != nil {
				merch = *m
			}
			out = append(out, map[string]any{"id": id, "type": typ, "amount": amt, "merchant": merch, "status": st, "at": ts})
		}
		c.JSON(200, out)
	})

	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"ok": true, "svc": "txn"}) })

	port := os.Getenv("PORT")
	if port == "" {
		port = "8003"
	}
	_ = r.Run(":" + port)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	b := []byte{}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
