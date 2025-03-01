package mongodb

import (
	"time"

	"github.com/charmbracelet/log"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EntitySecKill struct {
	EntityBase `bson:",inline"`

	ID primitive.ObjectID `json:"id,omitempty" bson:"_id,omitempty"`

	Name      string `json:"name" bson:"name"`
	Desc      string `json:"desc" bson:"desc"`
	StartTime int    `json:"start_time" bson:"start_time"`
	EndTime   int    `json:"end_time" bson:"end_time"`
	Total     int    `json:"total" bson:"total"`
}

// 实现 ModelEntier 接口
func (e *EntitySecKill) GetCreatedAt() time.Time {
	return e.CreatedAt
}

func (e *EntitySecKill) SetCreatedAt(t time.Time) {
	e.CreatedAt = t
}

var ModelSecKill *ModelBase[EntitySecKill, *EntitySecKill]

func init() {
	AddModelInitFunc(func(client *MongoClient) error {
		log.Info("init mongodb model seckill")

		collectionName := "seckill"

		ModelSecKill = NewModelBase[EntitySecKill, *EntitySecKill](collectionName)

		// // 检查索引是否存在
		// collection, err := mongoClient.GetCollection(collectionName)
		// if err != nil {
		// 	log.Error("Error mongoClient.GetCollection")
		// 	return err
		// }
		// usersIndexs, err := GetCollectionIndexs(context.Background(), collection)
		// if err != nil {
		// 	log.Error("Error GetCollectionIndexs")
		// 	return err
		// }
		// if !CheckCollectionCompoundIndexExists(usersIndexs, []string{"appid", "time"}, false) {
		// 	_, err = collection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		// 		Keys: bson.D{
		// 			{Key: "appid", Value: 1},
		// 			{Key: "time", Value: 1},
		// 		},
		// 		Options: options.Index().SetUnique(false),
		// 	})
		// 	if err != nil {
		// 		log.Error("Error CreateOne")
		// 		return err
		// 	}
		// }

		return nil
	})
}
