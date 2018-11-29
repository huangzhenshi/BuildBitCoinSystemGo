package main

import "fmt"

func (cli *CLI) createWallet() {
	//打开数据库里面存的钱包集合，如果没有就新建，如果有钱包了，就加载
	wallets, _ := NewWallets()
	//创建一个新钱包，并且添加到wallets里面去
	address := wallets.CreateWallet()
	//钱包集合的持久化
	wallets.SaveToFile()

	fmt.Printf("Your new address: %s\n", address)
}
