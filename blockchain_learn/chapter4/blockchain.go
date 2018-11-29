package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"os"

	"github.com/boltdb/bolt"
)

const dbFile = "blockchain.db"
const blocksBucket = "blocks"
const genesisCoinbaseData = "The Times 03/Jan/2009 Chancellor on brink of second bailout for banks"

type Blockchain struct {
	tip []byte
	db  *bolt.DB
}

type BlockchainIterator struct {
	currentHash []byte
	db          *bolt.DB
}

func (bc *Blockchain) MineBlock(transactions []*Transaction) {
	var lastHash []byte

	err := bc.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		lastHash = b.Get([]byte("l"))

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	newBlock := NewBlock(transactions, lastHash)

	err = bc.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		err := b.Put(newBlock.Hash, newBlock.Serialize())
		if err != nil {
			log.Panic(err)
		}

		err = b.Put([]byte("l"), newBlock.Hash)
		if err != nil {
			log.Panic(err)
		}

		bc.tip = newBlock.Hash

		return nil
	})
}

func (bc *Blockchain) Iterator() *BlockchainIterator {
	bci := &BlockchainIterator{bc.tip, bc.db}

	return bci
}

func (i *BlockchainIterator) Next() *Block {
	var block *Block

	err := i.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		encodedBlock := b.Get(i.currentHash)
		block = DeserializeBlock(encodedBlock)

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	i.currentHash = block.PrevBlockHash

	return block
}

func dbExists() bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}

	return true
}

// 创建一个有创世块的新链
func NewBlockchain(address string) *Blockchain {
	if dbExists() == false {
		fmt.Println("No existing blockchain found. Create one first.")
		os.Exit(1)
	}
	var tip []byte
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(blocksBucket))
		tip = b.Get([]byte("l"))

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{tip, db}

	return &bc
}

// CreateBlockchain 创建一个新的区块链数据库
// address 用来接收挖出创世块的奖励
func CreateBlockchain(address string) *Blockchain {
	if dbExists() {
		fmt.Println("Blockchain already exists.")
		os.Exit(1)
	}

	var tip []byte
	db, err := bolt.Open(dbFile, 0600, nil)
	if err != nil {
		log.Panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		cbtx := NewCoinbaseTX(address, genesisCoinbaseData)
		genesis := NewGenesisBlock(cbtx)

		b, err := tx.CreateBucket([]byte(blocksBucket))
		if err != nil {
			log.Panic(err)
		}

		err = b.Put(genesis.Hash, genesis.Serialize())
		if err != nil {
			log.Panic(err)
		}

		err = b.Put([]byte("l"), genesis.Hash)
		if err != nil {
			log.Panic(err)
		}
		tip = genesis.Hash

		return nil
	})

	if err != nil {
		log.Panic(err)
	}

	bc := Blockchain{tip, db}

	return &bc
}


// 这个方法会遍历区块链中所有的区块，以及每个区块里面的所有交易，以及每个交易里面的所有的输入和所有的输出
// 创建一个spentTXOs的map，里面存放了该用户所有的消费记录，消费的Txid作为key
// 创建一个unspentTXs的数组，是通过所有的该用户的入账记录 - spentTXOs的结果
// 最终得出unspentTXs，就是该用户所有的未花费输出的交易
func (bc *Blockchain) FindUnspentTransactions(address string) []Transaction {
	var unspentTXs []Transaction
	spentTXOs := make(map[string][]int)

	bci := bc.Iterator()
	//遍历区块链
	for {
		block := bci.Next()
		//遍历这个区块里面的所有交易
		//包括遍历这笔交易里面的所有的output，获取该用户的所有的入账记录
		//包括遍历完之后再遍历这笔交易里面的所有input，获取该用户的所有的消费记录，如果是创世块就不需要遍历了，因为input为nil
		for _, tx := range block.Transactions {
			txID := hex.EncodeToString(tx.ID)
		Outputs:
			//遍历这笔交易里面的所有的output
			for outIdx, out := range tx.Vout {
				//注意这里判断的是spentTXOs[]数组，就是所有的花费数组里面查找是否有消费记录
				// 因为这里的遍历是区块链倒序，所以肯定是消费一笔余额在产生一笔余额的前面
				//或者讲你只有有了钱，你才可以花钱，所以花钱的交易肯定在获取钱的前面，花了钱就把这个TXInput的 Txid存进去
				//所以有了下面的一个判断，判断spentTXOs[txID] != nil,如果为nil，表示该余额在后续的转账中没有被消费
				//如果不为nil，表示这笔余额已经被消费了
				if spentTXOs[txID] != nil {
					for _, spentOut := range spentTXOs[txID] {
						if spentOut == outIdx {
							continue Outputs
						}
					}
				}

				// 如果该交易输出可以被解锁，即可被花费，则append到结果集当中，
				if out.CanBeUnlockedWith(address) {
					unspentTXs = append(unspentTXs, *tx)
				}
			}
			//遍历这笔交易里面的所有input
			if tx.IsCoinbase() == false {
				for _, in := range tx.Vin {
					if in.CanUnlockOutputWith(address) {
						inTxID := hex.EncodeToString(in.Txid)
						spentTXOs[inTxID] = append(spentTXOs[inTxID], in.Vout)
					}
				}
			}
		}
		//找到创世块了，不用再往前面找了
		if len(block.PrevBlockHash) == 0 {
			break
		}
	}
	return unspentTXs
}

func (bc *Blockchain) FindUTXO(address string) []TXOutput {
	var UTXOs []TXOutput
	unspentTransactions := bc.FindUnspentTransactions(address)

	for _, tx := range unspentTransactions {
		for _, out := range tx.Vout {
			if out.CanBeUnlockedWith(address) {
				UTXOs = append(UTXOs, out)
			}
		}
	}
	return UTXOs
}

// FindSpendableOutputs 从 address 中找到至少 amount 的 UTXO
func (bc *Blockchain) FindSpendableOutputs(address string, amount int) (int, map[string][]int) {
	//这里的返回值是一个map，注意map的数据类型啊，这里的value是一个int类型的数组
	// key为这笔output对应的交易ID，也就是证明这笔余额是从哪里来的
	unspentOutputs := make(map[string][]int)

	//unspentTXs的类型是 []Transaction
	unspentTXs := bc.FindUnspentTransactions(address)
	accumulated := 0

Work:
	//根据unspentTXs，遍历里面的交易ID，获取到包含该用户结余未花费的交易
	for _, tx := range unspentTXs {
		txID := hex.EncodeToString(tx.ID)
		//遍历该笔交易的所有output，筛选出属于该用户的TXOutput，因为一笔交易中可能有多笔属于该用户的output
		//同时也有属于其他用户的结余记录
		//outIdx指的是 Vout的下标，因为 tx.Vout是一个数组，记住是下标，不是该笔Vout的值
		for outIdx, out := range tx.Vout {
			//如果余额未凑够，就继续凑，把这笔Output
			if out.CanBeUnlockedWith(address) && accumulated < amount {
				accumulated += out.Value
				//以一种数组的方式存储的 key为txID， value是一个数组，指向 这个[]Vout的下标
				unspentOutputs[txID] = append(unspentOutputs[txID], outIdx)
				if accumulated >= amount {
					break Work
				}
			}
		}
	}

	return accumulated, unspentOutputs
}
