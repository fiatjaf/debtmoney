package main

import (
	"github.com/graphql-go/graphql"
)

var queries = graphql.Fields{
	"user": &graphql.Field{
		Type: userType,
		Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			if p.Args["id"].(string) == "me" {
				userId, ok := p.Context.Value("userId").(string)
				if ok {
					return ensureUser(userId)
				}
			}
			return nil, nil
		},
	},
	"thing": &graphql.Field{
		Type: thingType,
		Args: graphql.FieldConfigArgument{
			"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			thingId := p.Args["id"].(string)

			var thing Thing
			err = pg.Get(&thing, `
SELECT * FROM things
WHERE id = $1
LIMIT 1
        `, thingId)
			if err != nil {
				log.Error().Str("thing", thingId).Err(err).Msg("on get thing")
			}

			return thing, err
		},
	},
}

var userType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "UserType",
		Fields: graphql.Fields{
			"id":      &graphql.Field{Type: graphql.String},
			"address": &graphql.Field{Type: graphql.String},
			"balances": &graphql.Field{
				Type: graphql.NewList(balanceType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					user := p.Source.(User)

					for i, b := range user.ha.Balances {
						var assetName string
						if b.Asset.Type == "native" {
							// continue // should we display this?
							assetName = "lumens"
						} else {
							issuerName := b.Asset.Issuer
							err := pg.Get(&issuerName,
								"SELECT id FROM users WHERE address = $1",
								b.Asset.Issuer)
							if err != nil {
								log.Error().
									Str("issuer", b.Asset.Issuer).
									Err(err).
									Msg("on asset issuer name query.")
							}
							assetName = b.Asset.Code + "#" + issuerName
						}

						user.Balances[i] = Balance{
							Asset:  assetName,
							Amount: b.Balance,
							Limit:  b.Limit,
						}
					}

					return user.Balances, nil
				},
			},
			"things": &graphql.Field{
				Type: graphql.NewList(thingType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					user := p.Source.(User)

					user.Things = []Thing{}
					err = pg.Select(&user.Things, `
SELECT things.* FROM things
INNER JOIN parties ON things.id = parties.thing_id
WHERE parties.thing_id = $1
ORDER BY thing_date
                    `, user.Id)
					if err != nil {
						log.Error().Str("user", user.Id).Err(err).
							Msg("on user things query")
						err = nil
					}

					return user.Things, err
				},
			},
		},
	},
)

var thingType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "ThingType",
		Fields: graphql.Fields{
			"id":         &graphql.Field{Type: graphql.String},
			"created_at": &graphql.Field{Type: graphql.String},
			"thing_date": &graphql.Field{Type: graphql.String},
			"name":       &graphql.Field{Type: graphql.String},
			"asset":      &graphql.Field{Type: graphql.String},
			"txn":        &graphql.Field{Type: graphql.String},
			"parties": &graphql.Field{
				Type: graphql.NewList(partyType),
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					thing := p.Source.(Thing)
					err := thing.fillParties()
					return thing.Parties, err
				},
			},
		},
	},
)

var partyType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "PartyType",
		Fields: graphql.Fields{
			"user_id":    &graphql.Field{Type: graphql.String},
			"thing_id":   &graphql.Field{Type: graphql.String},
			"paid":       &graphql.Field{Type: graphql.String},
			"due":        &graphql.Field{Type: graphql.String},
			"confirmed":  &graphql.Field{Type: graphql.Boolean},
			"registered": &graphql.Field{Type: graphql.Boolean},
		},
	},
)

var balanceType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "BalanceType",
		Fields: graphql.Fields{
			"asset":  &graphql.Field{Type: graphql.String},
			"amount": &graphql.Field{Type: graphql.String},
			"limit":  &graphql.Field{Type: graphql.String},
		},
	},
)

var mutations = graphql.Fields{
	"createThing": &graphql.Field{
		Type: resultType,
		Args: graphql.FieldConfigArgument{
			"thing_date": &graphql.ArgumentConfig{Type: graphql.String},
			"name":       &graphql.ArgumentConfig{Type: graphql.String},
			"asset": &graphql.ArgumentConfig{
				Type: graphql.NewNonNull(graphql.String),
			},
			"parties": &graphql.ArgumentConfig{
				Type: graphql.NewNonNull(graphql.NewList(
					graphql.NewNonNull(partyType),
				)),
			},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			userId, ok := p.Context.Value("userId").(string)
			if ok {
				return ensureUser(userId)
			}

			thingDate := p.Args["thing_date"].(string)
			name := p.Args["name"].(string)
			asset := p.Args["asset"].(string)
			parties := p.Args["parties"].([]interface{})

			log.Info().
				Str("date", thingDate).
				Str("name", name).
				Str("asset", asset).
				Int("nparties", len(parties)).
				Msg("thing creation")

			var thing Thing

			return Result{thing.Id}, nil
		},
	},
	"confirmThing": &graphql.Field{
		Type: thingType,
		Args: graphql.FieldConfigArgument{
			"thing_id": &graphql.ArgumentConfig{Type: graphql.String},
		},
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			userId, ok := p.Context.Value("userId").(string)
			if ok {
				return ensureUser(userId)
			}

			thingId := p.Args["thing_id"].(string)
			thing, published, err := confirmThing(thingId, userId)
			if err != nil {
				return nil, err
			}

			log.Info().
				Str("thing", thingId).
				Err(err).
				Bool("published", published).
				Msg("thing confirmation")

			return thing, err
		},
	},
}

var resultType = graphql.NewObject(
	graphql.ObjectConfig{
		Name: "ResultType",
		Fields: graphql.Fields{
			"value": &graphql.Field{Type: graphql.String},
		},
	},
)

type Result struct {
	Value string `json:"string"`
}

var rootQuery = graphql.ObjectConfig{Name: "RootQuery", Fields: queries}
var mutation = graphql.ObjectConfig{Name: "Mutation", Fields: mutations}

var schemaConfig = graphql.SchemaConfig{
	Query:    graphql.NewObject(rootQuery),
	Mutation: graphql.NewObject(mutation),
}
