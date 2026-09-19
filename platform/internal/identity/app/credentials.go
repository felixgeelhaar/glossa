package app

import (
	"context"
	"errors"
	"fmt"

	authgo "github.com/klarlabs-studio/auth-go/domain"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
)

// PasskeysEnabled reports whether a WebAuthn relying party is configured.
func (s *Service) PasskeysEnabled() bool { return s.passkeys != nil }

// BeginPasskeySignIn starts an assertion for the account's passkeys.
// state must round-trip to FinishPasskeySignIn unchanged; auth-go signs
// it, and the HTTP edge keeps it in an HttpOnly cookie.
func (s *Service) BeginPasskeySignIn(ctx context.Context, email string) (options, state []byte, err error) {
	if s.passkeys == nil {
		return nil, nil, ErrPasskeysDisabled
	}
	e, err := parseEmail(email)
	if err != nil {
		return nil, nil, err
	}
	var rec PersonRecord
	err = s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		var err error
		rec, err = st.PersonByEmail(ctx, e)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return nil, nil, ErrNoPasskeys
	}
	if err != nil {
		return nil, nil, err
	}
	options, state, err = s.passkeys.BeginLogin(ctx, userID(rec.ID))
	if err != nil {
		// The adapter translates auth-go's webauthn.ErrNoPasskeys.
		if errors.Is(err, ErrNoPasskeys) {
			return nil, nil, ErrNoPasskeys
		}
		return nil, nil, fmt.Errorf("identity: begin passkey sign-in: %w", err)
	}
	return options, state, nil
}

// FinishPasskeySignIn verifies the assertion and signs the person in.
func (s *Service) FinishPasskeySignIn(ctx context.Context, state, response []byte) (SignedIn, error) {
	if s.passkeys == nil {
		return SignedIn{}, ErrPasskeysDisabled
	}
	credID, err := s.passkeys.FinishLogin(ctx, state, response)
	if err != nil {
		s.logger.InfoContext(ctx, "passkey sign-in rejected", "error", err)
		return SignedIn{}, ErrPasskeyInvalid
	}
	var rec PersonRecord
	err = s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		person, err := st.PersonOfPasskey(ctx, credID)
		if err != nil {
			return err
		}
		rec, err = st.PersonByID(ctx, person)
		return err
	})
	if errors.Is(err, ErrNotFound) {
		return SignedIn{}, ErrPasskeyInvalid
	}
	if err != nil {
		return SignedIn{}, err
	}
	return s.completeSignIn(ctx, rec)
}

// BeginPasskeyRegistration starts enrolling a passkey for a person.
func (s *Service) BeginPasskeyRegistration(ctx context.Context, person domain.PersonID) (options, state []byte, err error) {
	if s.passkeys == nil {
		return nil, nil, ErrPasskeysDisabled
	}
	rec, err := s.Person(ctx, person)
	if err != nil {
		return nil, nil, err
	}
	options, state, err = s.passkeys.BeginRegistration(ctx, userID(person), rec.Email.String())
	if err != nil {
		return nil, nil, fmt.Errorf("identity: begin passkey registration: %w", err)
	}
	return options, state, nil
}

// Passkey is a registered passkey.
type Passkey struct {
	ID   []byte
	Name string
}

// FinishPasskeyRegistration verifies the attestation and stores the
// passkey under name.
func (s *Service) FinishPasskeyRegistration(ctx context.Context, person domain.PersonID, state, response []byte, name string) (Passkey, error) {
	if s.passkeys == nil {
		return Passkey{}, ErrPasskeysDisabled
	}
	cred, err := s.passkeys.FinishRegistration(ctx, state, response)
	if err != nil {
		s.logger.InfoContext(ctx, "passkey registration rejected", "error", err)
		return Passkey{}, ErrPasskeyInvalid
	}
	// The ceremony state names the user it was started for; it must be
	// the person finishing it.
	if owner, err := personOf(cred.UserID); err != nil || owner != person {
		return Passkey{}, ErrPasskeyInvalid
	}
	cred.Name = name
	err = s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		return st.AddPasskey(ctx, person, cred, s.now())
	})
	if err != nil {
		return Passkey{}, err
	}
	return Passkey{ID: cred.ID, Name: name}, nil
}

// TOTPEnrollment is a pending authenticator secret.
type TOTPEnrollment struct {
	Secret     string
	OTPAuthURI string
}

// BeginTOTP creates (or replaces) a pending TOTP secret. It takes effect
// once ConfirmTOTP proves the person's app produces codes for it.
func (s *Service) BeginTOTP(ctx context.Context, person domain.PersonID) (TOTPEnrollment, error) {
	rec, err := s.Person(ctx, person)
	if err != nil {
		return TOTPEnrollment{}, err
	}
	if rec.TOTPEnabled {
		return TOTPEnrollment{}, ErrTOTPAlreadyEnabled
	}
	secret, err := authgo.NewTOTPSecret()
	if err != nil {
		return TOTPEnrollment{}, err
	}
	err = s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		return st.PendingTOTP(ctx, person, secret, s.now())
	})
	if err != nil {
		return TOTPEnrollment{}, err
	}
	return TOTPEnrollment{Secret: secret.String(), OTPAuthURI: s.totpCfg.ProvisioningURI(secret, rec.Email.String())}, nil
}

// ConfirmTOTP turns a pending secret on. The code is checked (and its
// time step consumed) by auth-go's replay-safe TOTPService.
func (s *Service) ConfirmTOTP(ctx context.Context, person domain.PersonID, code string) error {
	rec, err := s.totpRecord(ctx, person)
	if errors.Is(err, ErrNotFound) || err == nil && rec.Confirmed {
		return ErrTOTPNotPending
	}
	if err != nil {
		return err
	}
	if err := s.totp.Verify(ctx, userID(person), code); err != nil {
		return totpError(err)
	}
	return s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		return st.ConfirmTOTP(ctx, person, s.now())
	})
}

// DisableTOTP turns TOTP off after checking a current code.
func (s *Service) DisableTOTP(ctx context.Context, person domain.PersonID, code string) error {
	rec, err := s.totpRecord(ctx, person)
	if errors.Is(err, ErrNotFound) || err == nil && !rec.Confirmed {
		return ErrTOTPNotEnabled
	}
	if err != nil {
		return err
	}
	if err := s.totp.Verify(ctx, userID(person), code); err != nil {
		return totpError(err)
	}
	return s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		return st.DeleteTOTP(ctx, person)
	})
}

func (s *Service) totpRecord(ctx context.Context, person domain.PersonID) (TOTPRecord, error) {
	var rec TOTPRecord
	err := s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		var err error
		rec, err = st.TOTP(ctx, person)
		return err
	})
	return rec, err
}

// Person loads a person.
func (s *Service) Person(ctx context.Context, id domain.PersonID) (PersonRecord, error) {
	var rec PersonRecord
	err := s.tx.InSystem(ctx, func(ctx context.Context, st SystemStore) error {
		var err error
		rec, err = st.PersonByID(ctx, id)
		return err
	})
	return rec, err
}
