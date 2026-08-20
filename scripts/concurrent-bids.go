// Demostración del motor concurrente de pujas de BidCraft.
//
// Lanza N pujas simultáneas del MISMO importe sobre la MISMA subasta. El motor
// debe aceptar exactamente una: las demás quedan por debajo del mínimo en cuanto
// la primera actualiza el precio.
//
// Uso:
//
//	go run scripts/concurrent-bids.go
//
// Variables de entorno (todas opcionales):
//
//	API_URL          por defecto http://localhost:8080
//	EMAIL            usuario con el que pujar (se registra si no existe)
//	PASSWORD         contraseña de ese usuario
//	AUCTION_ID       subasta a usar; si no se indica, el script crea una
//	CONCURRENT_BIDS  número de pujas simultáneas, por defecto 50
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

type auction struct {
	ID               int64  `json:"id"`
	CurrentPrice     int64  `json:"current_price"`
	MinimumIncrement int64  `json:"minimum_increment"`
	MinimumBid       int64  `json:"minimum_bid"`
	Status           string `json:"status"`
}

var (
	apiURL   = env("API_URL", "http://localhost:8080")
	email    = env("EMAIL", "concurrency@bidcraft.local")
	password = env("PASSWORD", "password123")
	client   = &http.Client{Timeout: 30 * time.Second}
)

func main() {
	token := login(email)

	target := auctionUnderTest(token)
	if target.Status != "ACTIVE" {
		fail("auction %d is %s; the demo needs an ACTIVE auction", target.ID, target.Status)
	}

	// Todas las goroutines envían este mismo importe: el mínimo aceptable ahora.
	amount := target.MinimumBid
	total := envInt("CONCURRENT_BIDS", 50)

	fmt.Printf("Auction: %d\n", target.ID)
	fmt.Printf("Initial price: %s\n", money(target.CurrentPrice))
	fmt.Printf("Minimum increment: %s\n", money(target.MinimumIncrement))
	fmt.Printf("Concurrent bids: %d\n", total)
	fmt.Printf("Bid amount: %s\n\n", money(amount))

	accepted, rejected, reasons := placeConcurrentBids(token, target.ID, amount, total)

	final := getAuction(target.ID)

	fmt.Printf("Accepted: %d\n", accepted)
	fmt.Printf("Rejected: %d\n", rejected)
	for reason, count := range reasons {
		fmt.Printf("  %s: %d\n", reason, count)
	}
	fmt.Printf("Final price: %s\n\n", money(final.CurrentPrice))

	// Las dos condiciones que demuestran que no hubo condición de carrera.
	if accepted != 1 {
		fail("expected exactly 1 accepted bid, got %d", accepted)
	}
	if final.CurrentPrice != amount {
		fail("expected the final price to be %s, got %s", money(amount), money(final.CurrentPrice))
	}

	fmt.Println("CONCURRENCY TEST PASSED")
}

// placeConcurrentBids lanza todas las pujas a la vez y cuenta los desenlaces.
func placeConcurrentBids(token string, auctionID, amount int64, total int) (int, int, map[string]int) {
	var mu sync.Mutex
	accepted, rejected := 0, 0
	reasons := make(map[string]int)

	// La barrera hace que las goroutines salgan juntas en vez de escalonadas.
	start := make(chan struct{})
	var ready, done sync.WaitGroup
	ready.Add(total)
	done.Add(total)

	for i := 0; i < total; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start

			status, reason := placeBid(token, auctionID, amount)

			mu.Lock()
			defer mu.Unlock()
			if status == http.StatusCreated {
				accepted++
				return
			}
			rejected++
			reasons[fmt.Sprintf("%d %s", status, reason)]++
		}()
	}

	ready.Wait()
	close(start)
	done.Wait()

	return accepted, rejected, reasons
}

func placeBid(token string, auctionID, amount int64) (int, string) {
	body, _ := json.Marshal(map[string]int64{"amount": amount})

	req, err := http.NewRequest(http.MethodPost,
		fmt.Sprintf("%s/api/v1/auctions/%d/bids", apiURL, auctionID), bytes.NewReader(body))
	if err != nil {
		fail("could not build the bid request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		fail("could not send the bid: %v", err)
	}
	defer resp.Body.Close()

	var payload struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)

	return resp.StatusCode, payload.Error
}

// login usa el usuario indicado y lo registra la primera vez.
func login(email string) string {
	credentials := map[string]string{"email": email, "password": password}

	status, body := post("/api/v1/auth/login", "", credentials)
	if status != http.StatusOK {
		signUp := map[string]string{"full_name": "Concurrency Bot", "email": email, "password": password}
		if status, body = post("/api/v1/auth/register", "", signUp); status != http.StatusCreated {
			fail("could not authenticate as %s: %s", email, body)
		}
	}

	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Token == "" {
		fail("the API did not return a token: %s", body)
	}

	return payload.Token
}

// auctionUnderTest usa AUCTION_ID si se indicó; si no, crea una subasta nueva
// para que la demostración parta siempre de un estado conocido.
// La subasta la publica una cuenta distinta: quien la crea no puede pujar por ella.
func auctionUnderTest(token string) auction {
	if raw := os.Getenv("AUCTION_ID"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			fail("AUCTION_ID must be a number, got %q", raw)
		}
		return getAuction(id)
	}

	seller := login("seller-" + email)

	status, body := post("/api/v1/auctions", seller, map[string]any{
		"title":             "Concurrency demo",
		"base_price":        10000, // $100.00 en centavos
		"minimum_increment": 1000,  // $10.00
		"duration_seconds":  300,
	})
	if status != http.StatusCreated {
		fail("could not create the auction: %s", body)
	}

	var created auction
	if err := json.Unmarshal(body, &created); err != nil {
		fail("could not read the created auction: %v", err)
	}

	return created
}

func getAuction(id int64) auction {
	resp, err := client.Get(fmt.Sprintf("%s/api/v1/auctions/%d", apiURL, id))
	if err != nil {
		fail("could not reach the API at %s: %v", apiURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		fail("could not read auction %d: %s", id, body)
	}

	var found auction
	if err := json.Unmarshal(body, &found); err != nil {
		fail("could not decode auction %d: %v", id, err)
	}

	return found
}

func post(path, token string, payload any) (int, []byte) {
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, apiURL+path, bytes.NewReader(body))
	if err != nil {
		fail("could not build the request to %s: %v", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		fail("could not reach the API at %s: %v", apiURL, err)
	}
	defer resp.Body.Close()

	response, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, response
}

// money convierte los centavos que usa la API en algo legible.
func money(cents int64) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func envInt(name string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(name))
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

// fail imprime el motivo y termina con código distinto de cero.
func fail(format string, args ...any) {
	fmt.Printf(format+"\n\n", args...)
	fmt.Println("CONCURRENCY TEST FAILED")
	os.Exit(1)
}
