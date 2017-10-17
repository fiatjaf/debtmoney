package main

import (
	b "github.com/stellar/go/build"
)

type DebtRecord struct {
	BaseRecord

	Debt Debt
	From User
	To   User
}

type Debt struct {
	From   string `json:"from"` // the one who owes
	To     string `json:"to"`   // the one who is owed
	Amount string `json:"amt"`
}

func instantiateDebtRecord(r BaseRecord) (d DebtRecord, err error) {
	d.BaseRecord = r

	err = r.Description.Unmarshal(&d.Debt)
	if err != nil {
		return
	}

	var u User
	if u, err = ensureUser(d.Debt.From); err == nil {
		d.From = u
	} else {
		return
	}
	if u, err = ensureUser(d.Debt.To); err == nil {
		d.To = u
	} else {
		return
	}

	return
}

func (d DebtRecord) shouldPublish() bool {
	var fromConfirmed bool
	var toConfirmed bool
	for _, uconfirmed := range d.Confirmed {
		if uconfirmed == d.Debt.From {
			fromConfirmed = true
		}
		if uconfirmed == d.Debt.To {
			toConfirmed = true
		}
	}
	if fromConfirmed && toConfirmed {
		return true
	}

	return false
}

func (d DebtRecord) publish() error {
	log.Info().Int("record", d.Id).Msg("publishing")

	// prepare the sql statement
	sql, err := pg.Prepare(`
UPDATE records
   SET transactions = array_append(transactions, $2)
 WHERE id = $1
    `)
	if err != nil {
		log.Warn().Err(err).Msg("failed to prepare append-transaction sql")
	}

	fundtotal := 0 // all the funds go to the receiver of the IOU

	fund, trustness, err := d.To.trust(d.From, d.Asset, d.Debt.Amount)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create trustline mutator")
		return err
	}
	if fund {
		fundtotal += 10
	}

	paymentness := b.Payment(
		b.SourceAccount{d.From.Address},
		b.Destination{d.To.Address},
		b.CreditAmount{d.Asset, d.From.Address, d.Debt.Amount},
	)

	fund, offerness, err := d.To.offer(
		d.From, d.Asset, d.To, d.Asset, "1", d.Debt.Amount)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create offer mutator")
		return err
	}
	if fund {
		fundtotal += 10
	}

	log.Info().Msg("publishing a single transaction")
	tx := createStellarTransaction()
	tx.Mutate(
		d.To.fund(fundtotal),
		trustness,
		paymentness,
		offerness,
	)
	hash, err := commitStellarTransaction(tx, s.SourceSeed, d.To.Seed, d.From.Seed)
	if err != nil {
		return err
	}

	// now commit the postgres transaction
	if err == nil {
		_, err = sql.Exec(d.Id, hash)

		if err != nil {
			log.Error().
				Err(err).
				Str("tx", hash).
				Msg("failed to append hash to postgres after stellar transaction ")
		}
	}

	return err
}
