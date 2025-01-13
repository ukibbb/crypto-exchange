package main

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/labstack/echo/v4"
	"github.com/ukibbb/crypto-exchange/orderbook"
)

func main() {
	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler
	ex, err := NewExchange(exchangePrivateKey)
	if err != nil {
		log.Fatal(err)
	}

	account := common.HexToAddress("0xD89596A710328F6e2970e97Ad341293509ddAA03")

	client, err := ethclient.Dial("http://localhost:7545")
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	balance, err := client.BalanceAt(ctx, account, nil)
	if err != nil {
		log.Fatal(err)
	}
	ethConversionFactor := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	// Since the division result may be fractional, convert big.Int to big.Float
	weiFloat := new(big.Float).SetInt(balance)
	ethConversionFactorFloat := new(big.Float).SetInt(ethConversionFactor) // Convert factor to big.Float
	eth := new(big.Float).Quo(weiFloat, ethConversionFactorFloat)

	fmt.Println("eth wei", eth, balance)

	privateKey, err := crypto.HexToECDSA("8185d90ca5d546283e99f33cad1d6c97e8ea707e2cf6bf1dd65ae8f7b53878f0")
	if err != nil {
		log.Fatal(err)
	}
	// Afterwards we need to get the account nonce.
	//  Every transaction requires a nonce.
	// A nonce by definition is a number that is only used once.
	//If it's a new account sending out a transaction then the nonce will be 0.
	//Every new transaction from an account must have a nonce that the previous nonce incremented by 1.
	// It's hard to keep manual track of all the nonces so
	// the ethereum client provides a helper method PendingNonceAt that will return the next nonce you should use.

	// The function requires the public address of the account we're sending from -- which we can derive from the private key.
	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("cannot assert type: publicKey is not of type *ecdsa.PublicKey")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		log.Fatal(err)
	}
	// Ether supports up to 18 decimal places so 1 ETH is 1 plus 18 zeros.
	// Here's a little tool to help you convert between ETH and wei:
	// The next step is to set the amount of ETH that we'll be transferring.
	// However we must convert ether to wei since that's what the Ethereum blockchain uses

	amount := big.NewInt(int64(1000000000000000000)) // in wei (1 eth)
	gasLimit := uint64(21000)                        // in units // gas Limit for standard eth transfer
	// gasPrice := big.NewInt(30000000000)     // in wei (30 gwei)
	// can vary based on market demand.
	// The go-ethereum client provides the SuggestGasPrice function for getting the average gas price based on x number of previous blocks.
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	toAddress := common.HexToAddress("0x71B4ef0D3632C6b4d9A4bEf27B8b0136DEF7EFa2")

	tx := types.NewTransaction(nonce, toAddress, amount, gasLimit, gasPrice, nil)
	// The next step is to sign the transaction with the private key of the sender.
	// To do this we call the SignTx method that takes
	// in the unsigned transaction and the private key that we constructed earlier.
	// The SignTx method requires the EIP155 signer, which we derive the chain ID from the client.

	// chainID, err := client.NetworkID(context.Background())
	// if err != nil {
	// 	log.Fatal(err)
	// }
	chainID := big.NewInt(1337)

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
	if err != nil {
		log.Fatal(err)
	}

	// Now we are finally ready to broadcast the transaction to the entire network
	//  by calling SendTransaction on the client which takes in the signed transaction.
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("tx sent: %s", signedTx.Hash().Hex())

	// func ethToWei(ethValue float64) *big.Int {
	// 	eth := big.NewFloat(ethValue)
	// 	ethConversionFactor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	// 	weiFloat := new(big.Float).Mul(eth, ethConversionFactor)
	// 	wei := new(big.Int)
	// 	weiFloat.Int(wei) // Convert back to big.Int
	// 	return wei
	// }

	e.Start(":3000")

	e.GET("/book/:market", ex.handleGetBook)
	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/order/:id", ex.handleCancelOrder)
}

func httpErrorHandler(err error, c echo.Context) {
	fmt.Println(err)
}

type OrderType string

const (
	MarketOrder OrderType = "MARKET"
	LimitOrder  OrderType = "LIMIT"
)

type Market string

const (
	MarketETH Market = "ETH"
)

const exchangePrivateKey string = "8185d90ca5d546283e99f33cad1d6c97e8ea707e2cf6bf1dd65ae8f7b53878f0"

func NewExchange(privateKey string) (*Exchange, error) {

	ecdsaPrivateKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	orderbooks := make(map[Market]*orderbook.OrderBook)
	orderbooks[MarketETH] = orderbook.NewOrderBook()
	return &Exchange{
		PrivateKey: ecdsaPrivateKey,
		orderBooks: orderbooks,
	}, nil
}

type Exchange struct {
	PrivateKey *ecdsa.PrivateKey
	orderBooks map[Market]*orderbook.OrderBook
}

type PlaceOrderRequest struct {
	Type   OrderType
	Bid    bool
	Size   float64
	Price  float64
	Market Market
}

type OrderBookData struct {
	TotalBidVolume float64
	TotalAskVolume float64
	Asks           []*Order
	Bids           []*Order
}

type Order struct {
	ID        int64
	Price     float64
	Size      float64
	Bid       bool
	Timestamp int64
}

type CancelOrderRequest struct {
	Bid bool
	ID  int64
}

type MatchedOrder struct {
	Price float64
	Size  float64
	ID    int64
}

func (ex *Exchange) handleCancelOrder(c echo.Context) error {

	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	ob := ex.orderBooks[MarketETH]
	order := ob.Orders[int64(id)]

	ob.CancelOrder(order)

	log.Println("order canceled id => ", id)

	return c.JSON(200, map[string]any{"msg": "order deleted"})
}

func (ex *Exchange) handleGetBook(c echo.Context) error {
	market := Market(c.Param("market"))

	ob, ok := ex.orderBooks[market]
	if !ok {
		return c.JSON(http.StatusBadRequest, map[string]any{"msg": "market not found"})
	}

	orderbookData := OrderBookData{
		TotalBidVolume: ob.BidTotalVolume(),
		TotalAskVolume: ob.AskTotalVolume(),
		Asks:           []*Order{},
		Bids:           []*Order{},
	}

	for _, limit := range ob.Asks() {
		for _, order := range limit.Orders {
			o := Order{
				ID:        order.ID,
				Price:     order.Limit.Price,
				Size:      order.Size,
				Bid:       order.Bid,
				Timestamp: order.Timestamp,
			}
			orderbookData.Asks = append(orderbookData.Asks, &o)

		}
	}

	for _, limit := range ob.Bids() {
		for _, order := range limit.Orders {
			o := Order{
				Price:     order.Limit.Price,
				Size:      order.Size,
				Bid:       order.Bid,
				Timestamp: order.Timestamp,
			}
			orderbookData.Bids = append(orderbookData.Bids, &o)

		}
	}

	return c.JSON(http.StatusOK, orderbookData)
}

func (ex *Exchange) handlePlaceOrder(c echo.Context) error {
	var placeOrderData PlaceOrderRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&placeOrderData); err != nil {
		return err
	}

	market := Market(placeOrderData.Market)
	ob := ex.orderBooks[market]
	order := orderbook.NewOrder(placeOrderData.Bid, placeOrderData.Size)

	if placeOrderData.Type == LimitOrder {
		ob.PlaceLimitOrder(placeOrderData.Price, order)
		return c.JSON(200, map[string]any{"msg": "limit order placed"})
	}

	if placeOrderData.Type == MarketOrder {
		matches := ob.PlaceMarketOrder(order)
		matchedOrders := make([]*MatchedOrder, len(matches))

		isBid := false
		if order.Bid {
			isBid = true
		}

		for i := 0; i < len(matchedOrders); i++ {
			id := matches[i].Bid.ID
			if isBid {
				id = matches[i].Ask.ID
			}
			matchedOrders[i] = &MatchedOrder{
				ID:    id,
				Size:  matches[i].SizeFilled,
				Price: matches[i].Price,
			}

		}

		return c.JSON(200, map[string]any{"msg": matchedOrders})

	}

	return nil
}
