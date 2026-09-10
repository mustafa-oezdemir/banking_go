// Package bootstrap initializes explicitly configured demo and administrator data.
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	sepa "github.com/mustafa-oezdemir/banking_go/internal/account"
	"github.com/mustafa-oezdemir/banking_go/internal/identity"
	"github.com/mustafa-oezdemir/banking_go/internal/ledger"
	"github.com/mustafa-oezdemir/banking_go/internal/payment"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
	"github.com/mustafa-oezdemir/banking_go/postgres/sqlc"
)

//nolint:govet // Group the user and both demo accounts by domain meaning.
type demoUser struct {
	user    sqlc.User
	current sqlc.Account
	savings sqlc.Account
}

// SeedDemoData creates a deterministic, fictional data set. Every write uses a
// unique constraint, a stable idempotency key, or an explicit existence check.
func SeedDemoData(ctx context.Context, store *db.Store, ledgerService *ledger.Service, payments *payment.Service) error {
	demoPassword := os.Getenv("DEMO_SEED_PASSWORD")
	if len(demoPassword) < 15 || len(demoPassword) > 72 {
		return errors.New("DEMO_SEED_PASSWORD must contain between 15 and 72 bytes")
	}
	demoHash, err := identity.HashPassword(demoPassword)
	if err != nil {
		return fmt.Errorf("hash demo seed password: %w", err)
	}

	helga, err := ensureDemoUser(ctx, store, "helga.müller@pehlione.com", "Helga Müller", 3100, demoHash)
	if err != nil {
		return err
	}
	jonas, err := ensureDemoUser(ctx, store, "jonas.schneider@pehlione.com", "Jonas Schneider", 3200, demoHash)
	if err != nil {
		return err
	}
	sofia, err := ensureDemoUser(ctx, store, "sofia.wagner@pehlione.com", "Sofia Wagner", 3300, demoHash)
	if err != nil {
		return err
	}
	if err = ensureBalance(ctx, store, ledgerService, helga.current, "4850.00"); err != nil {
		return err
	}
	if err = ensureBalance(ctx, store, ledgerService, jonas.current, "2200.00"); err != nil {
		return err
	}
	if err = ensureBalance(ctx, store, ledgerService, sofia.current, "1750.00"); err != nil {
		return err
	}

	rentIBAN, err := sepa.GermanDemoIBANForAccount(9000000001)
	if err != nil {
		return fmt.Errorf("seed rent IBAN: %w", err)
	}
	marketIBAN, err := sepa.GermanDemoIBANForAccount(9000000002)
	if err != nil {
		return fmt.Errorf("seed market IBAN: %w", err)
	}
	transitIBAN, err := sepa.GermanDemoIBANForAccount(9000000003)
	if err != nil {
		return fmt.Errorf("seed transit IBAN: %w", err)
	}
	streamIBAN, err := sepa.GermanDemoIBANForAccount(9000000004)
	if err != nil {
		return fmt.Errorf("seed streaming IBAN: %w", err)
	}
	for _, beneficiary := range []sqlc.CreateBeneficiaryParams{
		{OwnerID: helga.user.ID, Name: "Beispiel Hausverwaltung GmbH", Iban: rentIBAN, Category: nullString("Wohnen")},
		{OwnerID: helga.user.ID, Name: "Demo Markt Berlin", Iban: marketIBAN, Category: nullString("Lebensmittel")},
		{OwnerID: helga.user.ID, Name: "Demo Verkehrsbetriebe", Iban: transitIBAN, Category: nullString("Mobilität")},
		{OwnerID: helga.user.ID, Name: "Demo Streaming GmbH", Iban: streamIBAN, Category: nullString("Abonnements")},
	} {
		if _, err = store.CreateBeneficiary(ctx, beneficiary); err != nil {
			return fmt.Errorf("seed beneficiary: %w", err)
		}
	}

	paymentsToCreate := []payment.CreatePaymentInput{
		{OwnerID: helga.user.ID, SourceAccountID: helga.current.ID, BeneficiaryName: "Beispiel Hausverwaltung GmbH", BeneficiaryIBAN: rentIBAN, Amount: "980.00", TransferType: payment.PaymentStandard, ScheduleType: payment.ScheduleImmediate, Purpose: "Miete August", IdempotencyKey: "demo-seed-rent-v2"},
		{OwnerID: helga.user.ID, SourceAccountID: helga.current.ID, BeneficiaryName: "Demo Markt Berlin", BeneficiaryIBAN: marketIBAN, Amount: "86.40", TransferType: payment.PaymentStandard, ScheduleType: payment.ScheduleImmediate, Purpose: "Supermarkt", IdempotencyKey: "demo-seed-market-v2"}, //nolint:misspell // Correct German term.
		{OwnerID: helga.user.ID, SourceAccountID: helga.current.ID, BeneficiaryName: "Jonas Schneider", BeneficiaryIBAN: jonas.current.Iban, Amount: "24.50", TransferType: payment.PaymentInstant, ScheduleType: payment.ScheduleImmediate, Purpose: "Abendessen", IdempotencyKey: "demo-seed-instant-v2"},
		{OwnerID: helga.user.ID, SourceAccountID: helga.current.ID, BeneficiaryName: "Demo Verkehrsbetriebe", BeneficiaryIBAN: transitIBAN, Amount: "49.00", TransferType: payment.PaymentStandard, ScheduleType: payment.ScheduleScheduled, Purpose: "Deutschlandticket", RequestedExecution: time.Now().UTC().Add(72 * time.Hour), IdempotencyKey: "demo-seed-scheduled-v2"},
	}
	for _, input := range paymentsToCreate {
		if err = ensureSeedPayment(ctx, store, payments, input); err != nil {
			return err
		}
	}

	standing, err := store.ListStandingOrdersByOwner(ctx, helga.user.ID)
	if err != nil {
		return err
	}
	for _, order := range standing {
		if order.Purpose.Valid && order.Purpose.String == "Demo Streaming Abo" {
			return nil
		}
	}
	_, err = payments.CreateStandingOrder(ctx, payment.CreateStandingOrderInput{
		OwnerID: helga.user.ID, SourceAccountID: helga.current.ID,
		BeneficiaryName: "Demo Streaming GmbH", BeneficiaryIBAN: streamIBAN,
		Amount: "12.99", Purpose: "Demo Streaming Abo", TransferType: payment.PaymentStandard,
		Frequency: "MONTHLY", StartDate: time.Now().UTC().Add(24 * time.Hour),
	})
	return err
}

// SeedConfiguredAdmin provisions the explicitly configured administrator independently of demo data.
func SeedConfiguredAdmin(ctx context.Context, store *db.Store, ledgerService *ledger.Service) error {
	email := strings.ToLower(strings.TrimSpace(os.Getenv("ADMIN_SEED_EMAIL")))
	password := os.Getenv("ADMIN_SEED_PASSWORD")
	if email == "" && password == "" {
		return nil
	}
	if email == "" || password == "" {
		return errors.New("ADMIN_SEED_EMAIL and ADMIN_SEED_PASSWORD must be configured together")
	}
	if len(password) < 15 || len(password) > 72 {
		return errors.New("ADMIN_SEED_PASSWORD must contain between 15 and 72 bytes")
	}

	passwordHash := ""
	existing, lookupErr := store.GetUserByEmail(ctx, email)
	if lookupErr == nil && identity.VerifyPassword(existing.HashedPassword, password) {
		passwordHash = existing.HashedPassword
	} else if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("load configured admin: %w", lookupErr)
	}
	if passwordHash == "" {
		hash, hashErr := identity.HashPassword(password)
		if hashErr != nil {
			return fmt.Errorf("hash admin seed password: %w", hashErr)
		}
		passwordHash = hash
	}
	adminID, err := store.UpsertAdminUser(ctx, email, passwordHash, "Pehlione Administrator")
	if err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}

	accounts, err := store.ListAccountsByOwner(ctx, uuid.NullUUID{UUID: adminID, Valid: true})
	if err != nil {
		return fmt.Errorf("list admin accounts: %w", err)
	}
	var current sqlc.Account
	for _, account := range accounts {
		if account.AccountType == "GIROKONTO" {
			current = account
			break
		}
	}
	if current.ID == uuid.Nil {
		current, err = createConfiguredAdminAccount(ctx, store, adminID)
		if err != nil {
			return fmt.Errorf("seed admin account: %w", err)
		}
	}
	if err = ensureBalance(ctx, store, ledgerService, current, "500.00"); err != nil {
		return fmt.Errorf("seed admin balance: %w", err)
	}
	return nil
}

func createConfiguredAdminAccount(ctx context.Context, store *db.Store, ownerID uuid.UUID) (sqlc.Account, error) {
	iban, err := sepa.GenerateGermanDemoIBAN()
	if err != nil {
		return sqlc.Account{}, err
	}
	return store.CreateAccount(ctx, sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{UUID: ownerID, Valid: true}, Name: "Pehlione Admin Girokonto", Currency: "EUR",
		IsSystem: false, Iban: iban, AccountType: "GIROKONTO", Status: "ACTIVE",
	})
}

func ensureSeedPayment(ctx context.Context, store *db.Store, payments *payment.Service, input payment.CreatePaymentInput) error {
	order, err := store.GetPaymentOrderByIdempotency(ctx, sqlc.GetPaymentOrderByIdempotencyParams{
		OwnerID: input.OwnerID, IdempotencyKey: input.IdempotencyKey,
	})
	if err == sql.ErrNoRows {
		created, createErr := payments.CreatePayment(ctx, input)
		if createErr != nil {
			return fmt.Errorf("seed payment: %w", createErr)
		}
		order = created.Order
	} else if err != nil {
		return fmt.Errorf("find seed payment: %w", err)
	}

	if order.Status == payment.PaymentAwaitingConfirmation {
		if _, err = payments.ConfirmPayment(ctx, input.OwnerID, order.ID, order.VopResult != payment.VoPMatch); err != nil {
			return fmt.Errorf("confirm seed payment: %w", err)
		}
	}
	return nil
}

func ensureDemoUser(ctx context.Context, store *db.Store, email, fullName string, accountBase uint64, passwordHash string) (demoUser, error) {
	user, err := store.GetUserByEmail(ctx, email)
	if err == sql.ErrNoRows {
		created, createErr := store.CreateUser(ctx, sqlc.CreateUserParams{Email: email, HashedPassword: passwordHash, FullName: fullName})
		if createErr != nil {
			return demoUser{}, createErr
		}
		user, err = store.GetUserByID(ctx, created.ID)
	}
	if err != nil {
		return demoUser{}, err
	}
	accounts, err := store.ListAccountsByOwner(ctx, uuid.NullUUID{UUID: user.ID, Valid: true})
	if err != nil {
		return demoUser{}, err
	}
	result := demoUser{user: user}
	for _, account := range accounts {
		switch account.AccountType {
		case "GIROKONTO":
			result.current = account
		case "SPARKONTO":
			result.savings = account
		}
	}
	if result.current.ID == uuid.Nil {
		result.current, err = createSeedAccount(ctx, store, user.ID, fullName+" Girokonto", "GIROKONTO", accountBase)
		if err != nil {
			return demoUser{}, err
		}
	}
	if result.savings.ID == uuid.Nil {
		result.savings, err = createSeedAccount(ctx, store, user.ID, fullName+" Sparkonto", "SPARKONTO", accountBase+1)
	}
	return result, err
}

func createSeedAccount(ctx context.Context, store *db.Store, ownerID uuid.UUID, name, accountType string, number uint64) (sqlc.Account, error) {
	iban, err := sepa.GermanDemoIBANForAccount(number)
	if err != nil {
		return sqlc.Account{}, err
	}
	return store.CreateAccount(ctx, sqlc.CreateAccountParams{
		OwnerID: uuid.NullUUID{UUID: ownerID, Valid: true}, Name: name, Currency: "EUR",
		IsSystem: false, Iban: iban, AccountType: accountType, Status: "ACTIVE",
	})
}

func ensureBalance(ctx context.Context, store *db.Store, ledgerService *ledger.Service, account sqlc.Account, desired string) error {
	entries, err := store.ListEntriesByAccount(ctx, sqlc.ListEntriesByAccountParams{AccountID: account.ID, Limit: 1, Offset: 0})
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	current, err := decimal.NewFromString(account.Balance)
	if err != nil {
		return err
	}
	target := decimal.RequireFromString(desired)
	if current.GreaterThanOrEqual(target) {
		return nil
	}
	return ledgerService.Deposit(ctx, account.ID, target.Sub(current).StringFixed(2))
}

func nullString(value string) sql.NullString {
	value = strings.TrimSpace(value)
	return sql.NullString{String: value, Valid: value != ""}
}
