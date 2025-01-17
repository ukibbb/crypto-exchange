package server

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/labstack/echo/v4"
	"github.com/ukibbb/crypto-exchange/orderbook"
)

// Account nonce.
// Every transaction requires a nonce.
// A nonce by definition is a number that is only used once.
// If it's a new account sending out a transaction then the nonce will be 0.
// Every new transaction from an account must have a nonce that the previous nonce incremented by 1.
// the ethereum client provides a helper method PendingNonceAt that will return the next nonce you should use.

// The function requires the public address of the account we're sending from
// -- which we can derive from the private key.

func transferETH(client *ethclient.Client, fromPrivKey *ecdsa.PrivateKey, to common.Address, amount *big.Int) error {
	ctx := context.Background()
	publicKey := fromPrivKey.Public()
	publicKeyEcdsa, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("error casting publick key to ECDSA")
	}
	fromAddress := crypto.PubkeyToAddress(*publicKeyEcdsa)
	nonce, err := client.PendingNonceAt(ctx, fromAddress)
	if err != nil {
		return err
	}

	// amount := big.NewInt(int64(1000000000000000000)) // in wei (1 eth)
	gasLimit := uint64(21000) // in units // gas Limit for standard eth transfer
	// The go-ethereum client provides the SuggestGasPrice function
	// for getting the average gas price based on x number of previous blocks.
	gasPrice, err := client.SuggestGasPrice(context.Background()) // in wei (30 gwei)
	if err != nil {
		return err
	}
	// toAddress := common.HexToAddress("0x71B4ef0D3632C6b4d9A4bEf27B8b0136DEF7EFa2")
	tx := types.NewTransaction(nonce, to, amount, gasLimit, gasPrice, nil)
	chainID := big.NewInt(1337)
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), fromPrivKey)
	if err != nil {
		return err
	}
	return client.SendTransaction(ctx, signedTx)

}

// Ether supports up to 18 decimal places so 1 ETH is 1 plus 18 zeros.
// Etherum blockchain uses wei

// The next step is to sign the transaction with the private key of the sender.
// To do this we call the SignTx method that takes
// in the unsigned transaction and the private key that we constructed earlier.
// The SignTx method requires the EIP155 signer, which we derive the chain ID from the client.

// chainID, err := client.NetworkID(context.Background())

// Now we are finally ready to broadcast the transaction to the entire network
// by calling SendTransaction on the client which takes in the signed transaction.

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

func StartServer() {
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
	user := NewUser(pkStr, 5)
	ex.Users[user.ID] = user

	e.Start(":3000")

	e.POST("/order", ex.handlePlaceOrder)
	e.DELETE("/order/:id", ex.handleCancelOrder)

	e.GET("/order/:userID", ex.handleGetOrders)
	e.GET("/book/:market", ex.handleGetBook)
	e.GET("/book/:market/bid", ex.handleGetBestBid)
	e.GET("/book/:market/ask", ex.handleGetBestAsk)
}

type User struct {
	ID         int64
	PrivateKey *ecdsa.PrivateKey
}

func NewUser(privateKey string, id int64) *User {
	ecdsaPrivateKey, err := crypto.HexToECDSA(privateKey)
	if err != nil {
		panic(err)
	}

	return &User{
		ID:         id,
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
		Client: client,
		Users:  make(map[int64]*User),
		// orders:     make(map[int64]int64),
		Orders:     make(map[int64][]*orderbook.Order),
		PrivateKey: ecdsaPrivateKey,
		orderBooks: orderbooks,
	}, nil
}

type Exchange struct {
	Client *ethclient.Client

	mu    sync.RWMutex
	Users map[int64]*User

	Orders map[int64][]*orderbook.Order
	// orders     map[int64]int64
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
	UserID    int64
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
	UserID int64
	Price  float64
	Size   float64
	ID     int64
}

type PlaceOrderResponse struct {
	OrderID int64
}

type PriceResponse struct {
	Price float64
}

func (ex *Exchange) handleGetBestBid(c echo.Context) error {
	market := Market(c.Param("market"))
	ob := ex.orderBooks[market]
	if len(ob.Bids()) == 0 {
		return fmt.Errorf("the bids are empty")
	}
	bestBidPrice := ob.Bids()[0].Price

	pr := PriceResponse{
		Price: bestBidPrice,
	}

	return c.JSON(http.StatusOK, pr)

}
func (ex *Exchange) handleGetBestAsk(c echo.Context) error {
	market := Market(c.Param("market"))
	ob := ex.orderBooks[market]
	if len(ob.Asks()) == 0 {
		return fmt.Errorf("the asks are empty")
	}
	bestAskPrice := ob.Asks()[0].Price

	pr := PriceResponse{
		Price: bestAskPrice,
	}

	return c.JSON(http.StatusOK, pr)
}

type GetOrdersResponse struct {
	Asks []Order
	Bids []Order
}

func (ex *Exchange) handleGetOrders(c echo.Context) error {
	userIDStr := c.Param("userID")
	userID, err := strconv.Atoi(userIDStr)
	if err != nil {
		return err
	}

	ex.mu.RLock()
	defer ex.mu.RUnlock()
	orderBookOrders := ex.Orders[int64(userID)]
	ordersResp := &GetOrdersResponse{
		Asks: []Order{},
		Bids: []Order{},
	}

	for i := 0; i < len(orderBookOrders); i++ {
		// it could be that order is getting filled even
		// though it's included in this
		// response. Double check if the limit is not null
		if orderBookOrders[i].Limit == nil {
			continue
		}
		order := Order{
			UserID:    orderBookOrders[i].UserID,
			ID:        orderBookOrders[i].ID,
			Price:     orderBookOrders[i].Limit.Price,
			Size:      orderBookOrders[i].Size,
			Timestamp: orderBookOrders[i].Timestamp,
			Bid:       orderBookOrders[i].Bid,
		}

		if order.Bid {
			ordersResp.Bids = append(ordersResp.Bids, order)
		} else {
			ordersResp.Asks = append(ordersResp.Asks, order)
		}

	}

	return c.JSON(http.StatusOK, ordersResp)
}

func (ex *Exchange) handlePlaceMarketOrder(market Market, order *orderbook.Order) ([]orderbook.Match, []*MatchedOrder) {
	ob := ex.orderBooks[market]

	matches := ob.PlaceMarketOrder(order)
	matchedOrders := make([]*MatchedOrder, len(matches))

	isBid := false
	if order.Bid {
		isBid = true
	}

	totalSizeFilled := 0.0
	sumPrice := 0.0
	for i := 0; i < len(matchedOrders); i++ {
		id := matches[i].Bid.ID
		limitUserID := matches[i].Bid.UserID
		if isBid {
			limitUserID = matches[i].Ask.UserID
			id = matches[i].Ask.ID
		}
		matchedOrders[i] = &MatchedOrder{
			UserID: limitUserID,
			ID:     id,
			Size:   matches[i].SizeFilled,
			Price:  matches[i].Price,
		}

		totalSizeFilled += matches[i].SizeFilled
		sumPrice += matches[i].Price

	}
	avgPrice := sumPrice / float64(len(matches))
	_ = avgPrice

	newOrderMap := make(map[int64][]*orderbook.Order)

	ex.mu.RLock()

	for userID, orderbookOrders := range ex.Orders {
		for i := 0; i < len(orderbookOrders); i++ {
			// if the order is not filled we place it in the map copy
			// this means that size of the order = 0

			if !orderbookOrders[i].IsFilled() {

				newOrderMap[userID] = append(newOrderMap[userID], orderbookOrders[i])
			}

		}
	}

	ex.Orders = newOrderMap

	ex.mu.Unlock()

	return matches, matchedOrders

}

func (ex *Exchange) handlePlaceLimitOrder(market Market, price float64, o *orderbook.Order) error {
	ob := ex.orderBooks[market]
	ob.PlaceLimitOrder(price, o)

	ex.mu.Lock()
	ex.Orders[o.UserID] = append(ex.Orders[o.UserID], o)
	ex.mu.Unlock()

	// keep track of the user orders.

	// user, ok := ex.users[o.UserID]
	// if !ok {
	// 	return fmt.Errorf("user not found: %d", user.ID)
	// }

	// exchangePubKey := ex.PrivateKey.Public()
	// exchangePubKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)

	// if !ok {
	// 	return errors.New("error casting public key to ecdsa")
	// }

	// toAddress := crypto.PubkeyToAddress(*exchangePubKeyECDSA)

	// amount := big.NewInt(int64(o.Size))

	// return transferETH(ex.Client, user.PrivateKey, toAddress, amount)
	// transfer from users => exchange

	return nil
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
				UserID:    order.UserID,
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
				UserID:    order.UserID,
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

	// limit order
	if placeOrderData.Type == LimitOrder {
		if err := ex.handlePlaceLimitOrder(market, placeOrderData.Price, order); err != nil {
			return err
		}
	}

	// market order
	if placeOrderData.Type == MarketOrder {
		matches, _ := ex.handlePlaceMarketOrder(market, order)
		if err := ex.handleMatches(matches); err != nil {
			return err

		}

	}

	resp := &PlaceOrderResponse{
		OrderID: order.ID,
	}
	return c.JSON(200, resp)

}

func (ex *Exchange) handleMatches(matches []orderbook.Match) error {
	for _, match := range matches {
		fromUser, ok := ex.Users[match.Ask.UserID]
		if !ok {
			return fmt.Errorf("user not found %d", match.Ask.UserID)
		}

		toUser, ok := ex.Users[match.Bid.UserID]
		if !ok {
			return fmt.Errorf("user not found %d", match.Bid.UserID)
		}
		// this is only used for the fees.
		// exchangePubKey := ex.PrivateKey.Public()
		// exchangePubKeyECDSA, ok := exchangePubKey.(*ecdsa.PublicKey)

		// if !ok {
		// 	return errors.New("error casting public key to ecdsa")
		// }

		toAddress := crypto.PubkeyToAddress(toUser.PrivateKey.Public().(ecdsa.PublicKey))

		amount := big.NewInt(int64(match.SizeFilled))

		transferETH(ex.Client, fromUser.PrivateKey, toAddress, amount)

	}
	return nil
}
