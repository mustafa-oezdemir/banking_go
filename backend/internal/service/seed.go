package service

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
	"golang.org/x/crypto/bcrypt"

	"github.com/mustafa-oezdemir/banking_go/internal/db"
	"github.com/mustafa-oezdemir/banking_go/internal/sepa"
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
func SeedDemoData(ctx context.Context, store *db.Store, ledger *LedgerService, payments *PaymentService) error {
	demoPassword := os.Getenv("DEMO_SEED_PASSWORD")
	if len(demoPassword) < 15 || len(demoPassword) > 72 {
		return errors.New("DEMO_SEED_PASSWORD must contain between 15 and 72 bytes")
	}
	demoHash, err := bcrypt.GenerateFromPassword([]byte(demoPassword), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash demo seed password: %w", err)
	}

	anna, err := ensureDemoUser(ctx, store, "anna.beispiel@demo.invalid", "Anna Beispiel", 1000, string(demoHash))
	if err != nil {
		return err
	}
	maxUser, err := ensureDemoUser(ctx, store, "max.mustermann@demo.invalid", "Max Mustermann", 2000, string(demoHash))
	if err != nil {
		return err
	}
	if err = ensureBalance(ctx, ledger, anna.current, "4850.00"); err != nil {
		return err
	}
	if err = ensureBalance(ctx, ledger, maxUser.current, "2200.00"); err != nil {
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
		{OwnerID: anna.user.ID, Name: "Beispiel Hausverwaltung GmbH", Iban: rentIBAN, Category: nullString("Wohnen")},
		{OwnerID: anna.user.ID, Name: "Demo Markt Berlin", Iban: marketIBAN, Category: nullString("Lebensmittel")},
		{OwnerID: anna.user.ID, Name: "Demo Verkehrsbetriebe", Iban: transitIBAN, Category: nullString("Mobilität")},
		{OwnerID: anna.user.ID, Name: "Demo Streaming GmbH", Iban: streamIBAN, Category: nullString("Abonnements")},
	} {
		if _, err = store.CreateBeneficiary(ctx, beneficiary); err != nil {
			return fmt.Errorf("seed beneficiary: %w", err)
		}
	}

	paymentsToCreate := []CreatePaymentInput{
		{OwnerID: anna.user.ID, SourceAccountID: anna.current.ID, BeneficiaryName: "Beispiel Hausverwaltung GmbH", BeneficiaryIBAN: rentIBAN, Amount: "980.00", TransferType: PaymentStandard, ScheduleType: ScheduleImmediate, Purpose: "Miete August", IdempotencyKey: "demo-seed-rent-v1"},
		{OwnerID: anna.user.ID, SourceAccountID: anna.current.ID, BeneficiaryName: "Demo Markt Berlin", BeneficiaryIBAN: marketIBAN, Amount: "86.40", TransferType: PaymentStandard, ScheduleType: ScheduleImmediate, Purpose: "Supermarkt", IdempotencyKey: "demo-seed-market-v1"}, //nolint:misspell // Correct German term.
		{OwnerID: anna.user.ID, SourceAccountID: anna.current.ID, BeneficiaryName: "Max Mustermann", BeneficiaryIBAN: maxUser.current.Iban, Amount: "24.50", TransferType: PaymentInstant, ScheduleType: ScheduleImmediate, Purpose: "Abendessen", IdempotencyKey: "demo-seed-instant-v1"},
		{OwnerID: anna.user.ID, SourceAccountID: anna.current.ID, BeneficiaryName: "Demo Verkehrsbetriebe", BeneficiaryIBAN: transitIBAN, Amount: "49.00", TransferType: PaymentStandard, ScheduleType: ScheduleScheduled, Purpose: "Deutschlandticket", RequestedExecution: time.Now().UTC().Add(72 * time.Hour), IdempotencyKey: "demo-seed-scheduled-v1"},
	}
	for _, input := range paymentsToCreate {
		if err = ensureSeedPayment(ctx, store, payments, input); err != nil {
			return err
		}
	}

	standing, err := store.ListStandingOrdersByOwner(ctx, anna.user.ID)
	if err != nil {
		return err
	}
	for _, order := range standing {
		if order.Purpose.Valid && order.Purpose.String == "Demo Streaming Abo" {
			return nil
		}
	}
	_, err = payments.CreateStandingOrder(ctx, CreateStandingOrderInput{
		OwnerID: anna.user.ID, SourceAccountID: anna.current.ID,
		BeneficiaryName: "Demo Streaming GmbH", BeneficiaryIBAN: streamIBAN,
		Amount: "12.99", Purpose: "Demo Streaming Abo", TransferType: PaymentStandard,
		Frequency: "MONTHLY", StartDate: time.Now().UTC().Add(24 * time.Hour),
	})
	return err
}

// SeedConfiguredAdmin provisions the explicitly configured administrator independently of demo data.
func SeedConfiguredAdmin(ctx context.Context, store *db.Store, ledger *LedgerService) error {
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
	if lookupErr == nil && bcrypt.CompareHashAndPassword([]byte(existing.HashedPassword), []byte(password)) == nil {
		passwordHash = existing.HashedPassword
	} else if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
		return fmt.Errorf("load configured admin: %w", lookupErr)
	}
	if passwordHash == "" {
		hash, hashErr := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if hashErr != nil {
			return fmt.Errorf("hash admin seed password: %w", hashErr)
		}
		passwordHash = string(hash)
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
	if err = ensureBalance(ctx, ledger, current, "500.00"); err != nil {
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

func ensureSeedPayment(ctx context.Context, store *db.Store, payments *PaymentService, input CreatePaymentInput) error {
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

	if order.Status == PaymentAwaitingConfirmation {
		if _, err = payments.ConfirmPayment(ctx, input.OwnerID, order.ID, order.VopResult != VoPMatch); err != nil {
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

func ensureBalance(ctx context.Context, ledger *LedgerService, account sqlc.Account, desired string) error {
	entries, err := ledger.store.ListEntriesByAccount(ctx, sqlc.ListEntriesByAccountParams{AccountID: account.ID, Limit: 1, Offset: 0})
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
	return ledger.Deposit(ctx, account.ID, target.Sub(current).StringFixed(2))
}
