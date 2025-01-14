package main

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/labstack/echo/v4"
	"github.com/ukibbb/crypto-exchange/orderbook"
)

const (
	MarketOrder        OrderType = "MARKET"
	LimitOrder         OrderType = "LIMIT"
	exchangePrivateKey string    = "8185d90ca5d546283e99f33cad1d6c97e8ea707e2cf6bf1dd65ae8f7b53878f0"

	MarketETH Market = "ETH"
)

type (
	OrderType string
	Market    string
)

func main() {
	e := echo.New()
	e.HTTPErrorHandler = httpErrorHandler
	client, err := ethclient.Dial("http://localhost:7545")
	if err != nil {
		log.Fatal(err)
	}
	ex, err := NewExchange(exchangePrivateKey, client)
	if err != nil {
		log.Fatal(err)
	}

	// account := common.HexToAddress("0xD89596A710328F6e2970e97Ad341293509ddAA03")

	// ctx := context.Background()
	// balance, err := client.BalanceAt(ctx, account, nil)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// ethConversionFactor := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	// // Since the division result may be fractional, convert big.Int to big.Float
	// weiFloat := new(big.Float).SetInt(balance)
	// ethConversionFactorFloat := new(big.Float).SetInt(ethConversionFactor) // Convert factor to big.Float
	// eth := new(big.Float).Quo(weiFloat, ethConversionFactorFloat)

	// fmt.Println("eth wei", eth, balance)

	// privateKey, err := crypto.HexToECDSA("8185d90ca5d546283e99f33cad1d6c97e8ea707e2cf6bf1dd65ae8f7b53878f0")
	// if err != nil {
	// 	log.Fatal(err)
	// }

	// func ethToWei(ethValue float64) *big.Int {
	// 	eth := big.NewFloat(ethValue)
	// 	ethConversionFactor := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil))
	// 	weiFloat := new(big.Float).Mul(eth, ethConversionFactor)
	// 	wei := new(big.Int)
	// 	weiFloat.Int(wei) // Convert back to big.Int
	// 	return wei
	// }
	pkStr := "94be1fbf0c0b02d2196f7f3f610db86590435a8f773ef60aa5724d2154d6a506"
	pk, err := crypto.HexToECDSA(pkStr)
	if err != nil {
		log.Fatal(err)
	}
	user := &User{ID: 5, PrivateKey: pk}
	ex.users[user.ID] = user

	e.Start(":3000")

	e.GET("/book/:market", ex.handleGetBook)
	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/order/:id", ex.handleCancelOrder)
}

type User struct {
	ID         int64
	PrivateKey *ecdsa.PrivateKey
}

func NewUser(privateKey string) *User {
	ecdsaPrivateKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}

	return &User{
		PrivateKey: ecdsaPrivateKey,
	}
}

func httpErrorHandler(err error, c echo.Context) {
	fmt.Println(err)
}

func NewExchange(privateKey string, client *ethclient.Client) (*Exchange, error) {

	ecdsaPrivateKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		return nil, err
	}

	orderbooks := make(map[Market]*orderbook.OrderBook)
	orderbooks[MarketETH] = orderbook.NewOrderBook()
	return &Exchange{
		Client:     client,
		users:      make(map[int64]*User),
		orders:     make(map[int64]int64),
		PrivateKey: ecdsaPrivateKey,
		orderBooks: orderbooks,
	}, nil
}

type Exchange struct {
	Client     *ethclient.Client
	users      map[int64]*User
	orders     map[int64]int64
	PrivateKey *ecdsa.PrivateKey
	orderBooks map[Market]*orderbook.OrderBook
}

type PlaceOrderRequest struct {
	UserID int64
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

func (ex *Exchange) handlePlaceMarketOrder(market Market, order *orderbook.Order) ([]orderbook.Match, []*MatchedOrder) {
	ob := ex.orderBooks[market]

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
	return matches, matchedOrders

}

func (ex *Exchange) handlePlaceLimitOrder(market Market, price float64, o *orderbook.Order) error {
	ob := ex.orderBooks[market]
	ob.PlaceLimitOrder(price, o)

	user, ok := ex.users[o.UserID]
	if !ok {
		return fmt.Errorf("user not found: %d", user.ID)
	}

	exchangePubKey := ex.PrivateKey.Public()
	exchangePubKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)

	if !ok {
		return errors.New("error casting public key to ecdsa")
	}

	toAddress := crypto.PubkeyToAddress(*exchangePubKeyECDSA)

	amount := big.NewInt(int64(o.Size))

	return transferETH(ex.Client, user.PrivateKey, toAddress, amount)
	// transfer from users => exchange
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
	order := orderbook.NewOrder(placeOrderData.Bid, placeOrderData.Size, placeOrderData.UserID)

	if placeOrderData.Type == LimitOrder {
		if err := ex.handlePlaceLimitOrder(market, placeOrderData.Price, order); err != nil {
			return err
		}

		return c.JSON(200, map[string]any{"msg": "limit order placed"})
	}

	if placeOrderData.Type == MarketOrder {
		matches, matchedOrders := ex.handlePlaceMarketOrder(market, order)
		if err := ex.handleMatches(matches); err != nil {
			return err
		}

		return c.JSON(200, map[string]any{"msg": matchedOrders})

	}

	return nil
}

func (ex *Exchange) handleMatches(matches []orderbook.Match) error {
	return nil
}
