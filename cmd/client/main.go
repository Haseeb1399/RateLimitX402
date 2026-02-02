package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	x402 "github.com/coinbase/x402/go"
	x402http "github.com/coinbase/x402/go/http"
	evm "github.com/coinbase/x402/go/mechanisms/evm/exact/client"
	evmsigners "github.com/coinbase/x402/go/signers/evm"
)

func main() {
	// Create signer
	signer, err := evmsigners.NewClientSignerFromPrivateKey(os.Getenv("PRIVATE_KEY"))
	if err != nil {
		panic(fmt.Sprintf("Failed to create signer: %v", err))
	}

	// Configure client with builder pattern
	client := x402.Newx402Client().
		Register("eip155:*", evm.NewExactEvmScheme(signer))

	// Wrap HTTP client with payment handling
	httpClient := x402http.WrapHTTPClientWithPayment(
		http.DefaultClient,
		x402http.Newx402HTTPClient(client),
	)

	// Make request to paid endpoint (payment is handled automatically)
	for i := 1; i <= 10; i++ {
		func() {
			start := time.Now()
			resp, err := httpClient.Get("http://localhost:8081/cpu")
			if err != nil {
				fmt.Printf("Request %d error: %v\n", i, err)
				return
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)

			duration := time.Since(start)
			switch resp.StatusCode {
			case 200:
				fmt.Printf("Request %d: 200 OK (took %v)\n", i, duration)
				fmt.Println(string(body))
			default:
				fmt.Printf("Request %d: %d - %s (took %v)\n", i, resp.StatusCode, string(body), duration)
			}
		}()
	}
}
