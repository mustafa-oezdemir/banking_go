package card

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	sepa "github.com/mustafa-oezdemir/banking_go/internal/account"
	db "github.com/mustafa-oezdemir/banking_go/internal/platform/database"
)

var (
	ErrInvalidCard                = errors.New("invalid card credentials")
	ErrCardUnavailable            = errors.New("card is unavailable")
	ErrCardCredentialsUnavailable = errors.New("card credentials are unavailable")
)

type IssuedCard struct {
	ID         uuid.UUID `json:"id"`
	AccountID  uuid.UUID `json:"account_id"`
	CardNumber string    `json:"card_number"`
	CVC        string    `json:"cvc"`
	Brand      string    `json:"brand"`
	Last4      string    `json:"last4"`
	ExpMonth   int       `json:"exp_month"`
	ExpYear    int       `json:"exp_year"`
	Status     string    `json:"status"`
}

// RevealedCard is returned only to the authenticated owner on explicit request.
type RevealedCard struct {
	ID         uuid.UUID `json:"id"`
	AccountID  uuid.UUID `json:"account_id"`
	CardNumber string    `json:"card_number"`
	CVC        string    `json:"cvc"`
	Brand      string    `json:"brand"`
	Last4      string    `json:"last4"`
	ExpMonth   int       `json:"exp_month"`
	ExpYear    int       `json:"exp_year"`
}

type TokenizedCard struct {
	Token    string `json:"token"`
	Brand    string `json:"brand"`
	Last4    string `json:"last4"`
	ExpMonth int    `json:"exp_month"`
	ExpYear  int    `json:"exp_year"`
}

type AuthorizedCard struct {
	OwnerID   uuid.UUID
	AccountID uuid.UUID
	Brand     string
	Last4     string
}

type Service struct {
	store *db.Store
	key   []byte
	now   func() time.Time
}

func NewService(store *db.Store, key string) *Service {
	return &Service{store: store, key: []byte(strings.TrimSpace(key)), now: time.Now}
}

func (service *Service) ready() bool { return service.store != nil && len(service.key) >= 32 }

func (service *Service) Issue(ctx context.Context, ownerID, accountID uuid.UUID) (IssuedCard, error) {
	if !service.ready() || ownerID == uuid.Nil || accountID == uuid.Nil {
		return IssuedCard{}, ErrCardUnavailable
	}
	account, err := service.store.GetAccount(ctx, accountID)
	if err != nil || !sepa.CustomerCanOperate(ownerID, account.OwnerID.UUID, account.OwnerID.Valid, account.IsSystem) || account.Status != "ACTIVE" || account.Currency != "EUR" {
		return IssuedCard{}, ErrCardUnavailable
	}
	// Multiple virtual cards may be attached to the same active EUR account.
	// Each card gets independent credentials and a separate merchant token.
	cardID := uuid.New()
	pan, cvc := service.deriveCredentials(cardID)
	hash, err := bcrypt.GenerateFromPassword([]byte(cvc), bcrypt.DefaultCost)
	if err != nil {
		return IssuedCard{}, err
	}
	expires := service.now().UTC().AddDate(3, 0, 0)
	card, err := service.store.CreatePaymentCard(ctx, db.PaymentCard{
		ID:      cardID,
		OwnerID: ownerID, AccountID: accountID, PANFingerprint: service.fingerprint("pan:" + pan), LastFour: pan[len(pan)-4:], Brand: "visa",
		ExpMonth: int(expires.Month()), ExpYear: expires.Year(), CVCHash: string(hash), CredentialVersion: 1, Status: "ACTIVE",
	})
	if err != nil {
		return IssuedCard{}, err
	}
	return IssuedCard{ID: card.ID, AccountID: accountID, CardNumber: pan, CVC: cvc, Brand: card.Brand, Last4: card.LastFour, ExpMonth: card.ExpMonth, ExpYear: card.ExpYear, Status: "ACTIVE"}, nil
}

// Reveal derives credentials only for cards created under the versioned scheme.
// Neither raw PAN nor CVC is persisted.
func (service *Service) Reveal(ctx context.Context, ownerID, cardID uuid.UUID) (RevealedCard, error) {
	if !service.ready() || ownerID == uuid.Nil || cardID == uuid.Nil {
		return RevealedCard{}, ErrCardCredentialsUnavailable
	}
	stored, err := service.store.GetPaymentCardByIDAndOwner(ctx, cardID, ownerID)
	if err != nil || stored.CredentialVersion != 1 {
		return RevealedCard{}, ErrCardCredentialsUnavailable
	}
	pan, cvc := service.deriveCredentials(stored.ID)
	if !hmac.Equal(stored.PANFingerprint, service.fingerprint("pan:"+pan)) || bcrypt.CompareHashAndPassword([]byte(stored.CVCHash), []byte(cvc)) != nil {
		return RevealedCard{}, ErrCardCredentialsUnavailable
	}
	return RevealedCard{ID: stored.ID, AccountID: stored.AccountID, CardNumber: pan, CVC: cvc, Brand: stored.Brand, Last4: stored.LastFour, ExpMonth: stored.ExpMonth, ExpYear: stored.ExpYear}, nil
}

func (service *Service) List(ctx context.Context, ownerID uuid.UUID) ([]db.PaymentCard, error) {
	if !service.ready() || ownerID == uuid.Nil {
		return nil, ErrCardUnavailable
	}
	return service.store.ListPaymentCardsByOwner(ctx, ownerID)
}

func (service *Service) Tokenize(ctx context.Context, merchantID, pan, cvc string, month, year int) (TokenizedCard, error) {
	pan, cvc, merchantID = normalizePAN(pan), strings.TrimSpace(cvc), strings.ToLower(strings.TrimSpace(merchantID))
	if !service.ready() || !validPAN(pan) || !validCVC(cvc) {
		return TokenizedCard{}, ErrInvalidCard
	}
	merchant, err := service.store.GetMerchant(ctx, merchantID)
	if err != nil || !merchant.Active {
		return TokenizedCard{}, ErrInvalidCard
	}
	stored, err := service.store.GetPaymentCardByFingerprint(ctx, service.fingerprint("pan:"+pan))
	if err != nil || !service.usable(stored, month, year) || bcrypt.CompareHashAndPassword([]byte(stored.CVCHash), []byte(cvc)) != nil {
		return TokenizedCard{}, ErrInvalidCard
	}
	rawToken := make([]byte, 32)
	if _, err := rand.Read(rawToken); err != nil {
		return TokenizedCard{}, err
	}
	token := hex.EncodeToString(rawToken)
	if err := service.store.UpsertMerchantCardToken(ctx, merchantID, stored.ID, service.fingerprint("token:"+token)); err != nil {
		return TokenizedCard{}, err
	}
	return TokenizedCard{Token: token, Brand: stored.Brand, Last4: stored.LastFour, ExpMonth: stored.ExpMonth, ExpYear: stored.ExpYear}, nil
}

func (service *Service) Authorize(ctx context.Context, merchantID, token, cvc string) (AuthorizedCard, error) {
	merchantID, token, cvc = strings.ToLower(strings.TrimSpace(merchantID)), strings.TrimSpace(token), strings.TrimSpace(cvc)
	if !service.ready() || len(token) != 64 || !validCVC(cvc) {
		return AuthorizedCard{}, ErrInvalidCard
	}
	if _, err := hex.DecodeString(token); err != nil {
		return AuthorizedCard{}, ErrInvalidCard
	}
	stored, err := service.store.GetCardByMerchantToken(ctx, merchantID, service.fingerprint("token:"+token))
	if err != nil || !service.usable(stored.Card, stored.Card.ExpMonth, stored.Card.ExpYear) || bcrypt.CompareHashAndPassword([]byte(stored.Card.CVCHash), []byte(cvc)) != nil {
		return AuthorizedCard{}, ErrInvalidCard
	}
	if err := service.store.TouchMerchantCardToken(ctx, stored.ID); err != nil {
		return AuthorizedCard{}, err
	}
	return AuthorizedCard{OwnerID: stored.Card.OwnerID, AccountID: stored.Card.AccountID, Brand: stored.Card.Brand, Last4: stored.Card.LastFour}, nil
}

func (service *Service) usable(card db.PaymentCard, month, year int) bool {
	now := service.now().UTC()
	return card.Status == "ACTIVE" && card.ExpMonth == month && card.ExpYear == year && (year > now.Year() || year == now.Year() && month >= int(now.Month()))
}

func (service *Service) fingerprint(value string) []byte {
	mac := hmac.New(sha256.New, service.key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func (service *Service) deriveCredentials(cardID uuid.UUID) (string, string) {
	panMAC := hmac.New(sha256.New, service.key)
	_, _ = panMAC.Write([]byte("pan:" + cardID.String()))
	digits := panMAC.Sum(nil)
	base := make([]byte, 15)
	base[0] = '4'
	for index := 1; index < len(base); index++ {
		base[index] = '0' + digits[index-1]%10
	}
	for digit := byte('0'); digit <= '9'; digit++ {
		candidate := string(base) + string(digit)
		if validPAN(candidate) {
			cvcMAC := hmac.New(sha256.New, service.key)
			_, _ = cvcMAC.Write([]byte("cvc:" + cardID.String()))
			cvcBytes := cvcMAC.Sum(nil)
			return candidate, fmt.Sprintf("%03d", (int(cvcBytes[0])<<8|int(cvcBytes[1]))%1000)
		}
	}
	panic("unreachable: Luhn check digit must exist")
}

func normalizePAN(value string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(strings.TrimSpace(value))
}
func validCVC(value string) bool {
	return len(value) == 3 && value[0] >= '0' && value[0] <= '9' && value[1] >= '0' && value[1] <= '9' && value[2] >= '0' && value[2] <= '9'
}
func validPAN(value string) bool {
	if len(value) != 16 || value[0] != '4' {
		return false
	}
	sum := 0
	for index, char := range value {
		if char < '0' || char > '9' {
			return false
		}
		digit := int(char - '0')
		if index%2 == 0 {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}
		sum += digit
	}
	return subtle.ConstantTimeByteEq(byte(sum%10), 0) == 1
}

func generatePAN() (string, error) {
	prefix, err := randomDigits(14)
	if err != nil {
		return "", err
	}
	base := "4" + prefix
	for digit := byte('0'); digit <= '9'; digit++ {
		candidate := base + string(digit)
		if validPAN(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("generate card checksum")
}

func randomDigits(length int) (string, error) {
	result := make([]byte, length)
	for index := range result {
		var value [1]byte
		for {
			if _, err := rand.Read(value[:]); err != nil {
				return "", err
			}
			if value[0] < 250 {
				result[index] = '0' + value[0]%10
				break
			}
		}
	}
	return string(result), nil
}
