package main

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
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
