package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

// The receipt payload structure
type Receipt struct {
	Retailer     string  `json:"retailer"`
	PurchaseDate string  `json:"purchaseDate"`
	PurchaseTime string  `json:"purchaseTime"`
	Total        string  `json:"total"`
	Items        []Item  `json:"items"`
}

// A single item in the receipt
type Item struct {
	ShortDescription string `json:"shortDescription"`
	Price            string `json:"price"`
}

// Response for POST /receipts/process
type ProcessResponse struct {
	ID string `json:"id"`
}

// Response for GET /receipts/{id}/points
type PointsResponse struct {
	Points int `json:"points"`
}

// The storage for the points in memory
var (
	receiptStore = make(map[string]int)
	storeMutex   = sync.RWMutex{}
)

func main() {
	// Using Gorilla Mux for URL routing.
	r := mux.NewRouter()
	r.HandleFunc("/receipts/process", processReceiptHandler).Methods("POST")
	r.HandleFunc("/receipts/{id}/points", getPointsHandler).Methods("GET")
	port := "8080"
	log.Printf("Listening on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

// processReceiptHandler handles POST /receipts/process
func processReceiptHandler(w http.ResponseWriter, r *http.Request) {
	var receipt Receipt

	// Decoding JSON into the struct we made
	if err := json.NewDecoder(r.Body).Decode(&receipt); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Calculating points based on rules
	points, err := calculatePoints(receipt)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error calculating points: %v", err), http.StatusBadRequest)
		return
	}

	// Generate unique ID for the receipt.
	id := uuid.New().String()

	// Store the calculated points in the in-memory map.
	storeMutex.Lock()
	receiptStore[id] = points
	storeMutex.Unlock()

	// Return the receipt ID.
	resp := ProcessResponse{ID: id}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// getPointsHandler handles GET /receipts/{id}/points
func getPointsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	storeMutex.RLock()
	points, exists := receiptStore[id]
	storeMutex.RUnlock()

	if !exists {
		http.Error(w, "Receipt not found", http.StatusNotFound)
		return
	}

	resp := PointsResponse{Points: points}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// calculatePoints applies the business rules to calculate points for a receipt.
func calculatePoints(receipt Receipt) (int, error) {
	totalPoints := 0

	// One point for every alphanumeric character in the retailer name.
	re := regexp.MustCompile(`[A-Za-z0-9]`)
	alphaNumChars := re.FindAllString(receipt.Retailer, -1)
	totalPoints += len(alphaNumChars)

	// Parse the string into a float.
	totalFloat, err := strconv.ParseFloat(receipt.Total, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid total")
	}

	// 50 points if the total is a round dollar amount with no cents.
	if math.Mod(totalFloat, 1.0) == 0 {
		totalPoints += 50
	}

	// 25 points if the total is a multiple of 0.25.
	if math.Mod(totalFloat, 0.25) == 0 {
		totalPoints += 25
	}

	// 5 points for every two items on the receipt.
	totalPoints += (len(receipt.Items) / 2) * 5

	// if item trimmed length of the short description is a multiple of 3 add the multiply of price by 0.2 and round up to the nearest integer
	for _, item := range receipt.Items {
		desc := strings.TrimSpace(item.ShortDescription)
		if len(desc)%3 == 0 {
			priceFloat, err := strconv.ParseFloat(item.Price, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid item price")
			}
			// Calculate points: price * 0.2 then round up.
			itemPoints := int(math.Ceil(priceFloat * 0.2))
			totalPoints += itemPoints
		}
	}

	// 6 points if the day in the purchase date is odd.
	// Expecting date in YYYY-MM-DD format.
	date, err := time.Parse("2006-01-02", receipt.PurchaseDate)
	if err != nil {
		return 0, fmt.Errorf("invalid purchaseDate")
	}
	if date.Day()%2 == 1 {
		totalPoints += 6
	}

	// 10 points if the time of purchase is after 2:00pm and before 4:00pm.
	// Expecting time in HH:MM (24-hour) format.
	purchaseTime, err := time.Parse("15:04", receipt.PurchaseTime)
	if err != nil {
		return 0, fmt.Errorf("invalid purchaseTime")
	}
	// Create fixed times for 14:00 and 16:00.
	afterTwo, _ := time.Parse("15:04", "14:00")
	beforeFour, _ := time.Parse("15:04", "16:00")
	if purchaseTime.After(afterTwo) && purchaseTime.Before(beforeFour) {
		totalPoints += 10
	}

	return totalPoints, nil
}
