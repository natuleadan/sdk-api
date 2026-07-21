package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type mongoField struct {
	Name     string
	GoType   string
	BSONName string
	JSONName string
}

func runModelFromMongo(uri, dbName, collectionName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("mongo connect: %w", err)
	}
	defer client.Disconnect(ctx) //nolint:errcheck

	coll := client.Database(dbName).Collection(collectionName)
	var doc bson.M
	if err := coll.FindOne(ctx, bson.M{}).Decode(&doc); err != nil {
		return fmt.Errorf("find document: %w", err)
	}

	modelName := pascalCase(singular(collectionName))
	fmt.Printf("// Model generated from MongoDB collection: %s\n", collectionName)
	fmt.Printf("type %s struct {\n", modelName)
	fmt.Printf("\tID bson.ObjectID `bson:\"_id,omitempty\" json:\"id\"`\n")

	for k, v := range doc {
		keyUpper := strings.ToUpper(k)
		if keyUpper == "_ID" || keyUpper == "ID" {
			continue
		}
		f := mongoFieldToGo(k, v)
		if f != nil {
			fmt.Printf("\t%s %s `bson:\"%s\" json:\"%s\"`\n", f.Name, f.GoType, f.BSONName, f.JSONName)
		}
	}

	fmt.Printf("\tCreatedAt time.Time `bson:\"created_at,omitempty\" json:\"created_at,omitempty\"`\n")
	fmt.Printf("\tUpdatedAt time.Time `bson:\"updated_at,omitempty\" json:\"updated_at,omitempty\"`\n")
	fmt.Printf("}\n")
	return nil
}

func mongoFieldToGo(k string, v any) *mongoField {
	name := pascalCase(k)
	jsonName := toSnake(k)
	bsonName := k

	var goType string
	switch val := v.(type) {
	case string:
		goType = "string"
	case bool:
		goType = "bool"
	case int32:
		goType = "int32"
	case int64:
		goType = "int64"
	case float64:
		goType = "float64"
	case bson.M, map[string]any:
		goType = "any"
	case bson.A, []any:
		goType = "[]any"
	case time.Time:
		goType = "time.Time"
	case bson.ObjectID:
		goType = "bson.ObjectID"
	case nil:
		goType = "any"
	default:
		goType = fmt.Sprintf("%T", val)
	}

	return &mongoField{
		Name:     name,
		GoType:   goType,
		BSONName: bsonName,
		JSONName: jsonName,
	}
}
