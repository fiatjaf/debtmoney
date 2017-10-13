package main

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/jmoiron/sqlx/types"
	"github.com/kr/pretty"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	b "github.com/stellar/go/build"
	"github.com/stellar/go/clients/horizon"
	"github.com/stellar/go/keypair"
)

type User struct {
	Id       string       `json:"id"       db:"id"`
	Address  string       `json:"address"  db:"address"`
	Seed     string       `json:"-"        db:"seed"`
	Balances []Balance    `json:"balances" db:"-"`
	Records  []BaseRecord `json:"records"  db:"-"`

	ha horizon.Account
}

func ensureUser(id string) (user User, err error) {
	id = strings.ToLower(id)

	txn, err := pg.Beginx()
	if err != nil {
		return
	}
	defer txn.Rollback()

	log.Info().Str("id", id).Msg("checking account existence")
	err = txn.Get(&user, `
SELECT * FROM users
 WHERE id = $1
    `, id)
	if err != nil && err.Error() != "sql: no rows in result set" {
		return
	}

	// load account info from stellar
	// runs no matter what
	defer func() {
		if err == nil {
			var ha horizon.Account
			ha, err = h.LoadAccount(user.Address)
			user.ha = ha
		}
	}()

	if user.Id != "" {
		// ok, we've found a row
		return
	}

	// proceed to create a new row
	log.Info().Str("id", id).Msg("creating account")
	pair, err := keypair.Random()
	if err != nil {
		return
	}

	_, err = txn.Exec(`
INSERT INTO users (id, address, seed)
VALUES ($1, $2, $3)
    `, id, pair.Address(), pair.Seed())
	if err != nil {
		return
	}

	txn.Commit()

	user = User{
		Id:      id,
		Address: pair.Address(),
		Seed:    pair.Seed(),
	}
	err = user.fundInitial()
	if err != nil {
		return
	}

	return
}

func (user User) fundInitial() error {
	tx := b.Transaction(
		n,
		b.SourceAccount{s.SourceAddress},
		b.AutoSequence{h},
		b.CreateAccount(
			b.Destination{user.Address},
			b.NativeAmount{"20.1"},
		),
	)
	if tx.Err != nil {
		return tx.Err
	}

	txe := tx.Sign(s.SourceSeed)
	blob, err := txe.Base64()
	if err != nil {
		return err
	}

	_, err = h.SubmitTransaction(blob)
	if err != nil {
		return err
	}
	return nil
}

func (user User) fundMore(amount int) error {
	tx := b.Transaction(
		n,
		b.SourceAccount{s.SourceAddress},
		b.AutoSequence{h},
		b.Payment(
			b.Destination{user.Address},
			b.NativeAmount{strconv.Itoa(amount)},
		),
	)
	if tx.Err != nil {
		return tx.Err
	}

	txe := tx.Sign(s.SourceSeed)
	blob, err := txe.Base64()
	if err != nil {
		return err
	}

	_, err = h.SubmitTransaction(blob)
	if err != nil {
		return err
	}
	return nil
}

func (rec User) trust(iss User, asset string, newAmount string) error {
	newAmountd, err := decimal.NewFromString(newAmount)
	if err != nil {
		return err
	}

	zero := decimal.Decimal{}
	fund := true
	need := newAmountd

	for _, balance := range rec.ha.Balances {
		if balance.Asset.Issuer == iss.Address && balance.Asset.Code == asset {
			// asset already in the balance
			fund = false

			// adjust trustline amount
			limit, err1 := decimal.NewFromString(balance.Limit)
			balance, err2 := decimal.NewFromString(balance.Balance)
			if err1 != nil || err2 != nil {
				return errors.New("wrong balance values received from horizon")
			}
			free := limit.Sub(balance)
			if free.GreaterThan(newAmountd) {
				need = zero
			} else {
				need = newAmountd.Add(limit).Sub(free)
			}
		}
	}

	log.Debug().
		Str("truster", rec.Id).
		Str("trustee", iss.Id).
		Str("need", need.String()).
		Bool("fund", fund).
		Msg("adjusting trustline")

	if need.Equals(zero) {
		return nil
	}

	if fund {
		err := rec.fundMore(10)
		if err != nil {
			return err
		}
	}

	// change or create the trustline
	tx := b.Transaction(
		n,
		b.SourceAccount{rec.Address},
		b.AutoSequence{h},
		b.Trust(asset, iss.Address, b.Limit(need.StringFixed(2))),
	)
	if tx.Err != nil {
		return tx.Err
	}

	txe := tx.Sign(rec.Seed)
	blob, err := txe.Base64()
	if err != nil {
		return err
	}

	_, err = h.SubmitTransaction(blob)
	if err != nil {
		if herr, ok := err.(*horizon.Error); ok {
			c, _ := herr.ResultCodes()
			pretty.Log(c)
		}
		return err
	}
	return nil
}

func (me User) createDebt(from, to, assetCode, amount string) (*BaseRecord, error) {
	var r BaseRecord

	desc, _ := json.Marshal(Debt{
		From:   from,
		To:     to,
		Amount: amount,
	})

	err := pg.Get(&r, `
INSERT INTO records (kind, description, asset, confirmed)
VALUES ('debt', $1, $2, $3)
RETURNING *
    `, types.JSONText(desc), assetCode, StringSlice{me.Id})

	return &r, err
}

type Balance struct {
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Limit  string `json:"limit"`
}
