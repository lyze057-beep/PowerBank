package pkg

import "fmt"

const (
	walletBalanceKeyPattern = "wallet:balance:%s"
)

// BuildWalletBalanceKey builds redis key for user wallet balance cache.
func BuildWalletBalanceKey(uid string) string {
	return fmt.Sprintf(walletBalanceKeyPattern, uid)
}
