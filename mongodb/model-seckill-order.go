package mongodb

import (
	"context"
	"time"

	"github.com/charmbracelet/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type EntitySecKillOrder struct {
	EntityBase `bson:",inline"`

	ID primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`

	SeckillID string `json:"seckill_id" bson:"seckill_id"`
	UserID    string `json:"user_id" bson:"user_id"`

	SecKillTime *time.Time `json:"seckill_time" bson:"seckill_time"`
}

// 实现 ModelEntier 接口
func (e *EntitySecKillOrder) GetCreatedAt() time.Time {
	return e.CreatedAt
}

func (e *EntitySecKillOrder) SetCreatedAt(t time.Time) {
	e.CreatedAt = t
}

var ModelSecKillOrder *ModelBase[EntitySecKillOrder, *EntitySecKillOrder]

func init() {
	AddModelInitFunc(func(client *MongoClient) error {
		log.Info("init mongodb model seckill_order")

		collectionName := "seckill_order"

		ModelSecKillOrder = NewModelBase[EntitySecKillOrder, *EntitySecKillOrder](collectionName)

		// 检查索引是否存在
		collection, err := mongoClient.GetCollection(collectionName)
		if err != nil {
			log.Error("Error mongoClient.GetCollection")
			return err
		}
		usersIndexs, err := GetCollectionIndexs(context.Background(), collection)
		if err != nil {
			log.Error("Error GetCollectionIndexs")
			return err
		}
		if !CheckCollectionCompoundIndexExists(usersIndexs, []string{"seckill_id", "user_id"}, false) {
			_, err = collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
				Keys: bson.D{
					{Key: "seckill_id", Value: 1},
					{Key: "user_id", Value: 1},
				},
				Options: options.Index().SetUnique(true),
			})
			if err != nil {
				log.Error("Error Create Index")
				return err
			}
		}

		return nil
	})
}
