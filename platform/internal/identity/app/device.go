package app

import (
	"context"
	"errors"
	"time"

	"github.com/felixgeelhaar/glossa/platform/internal/identity/domain"
)

// Device sign-in (RFC 0006 §7.2; the OAuth 2.0 device authorization
// grant, RFC 8628): how a person signs the CLI in as themselves, which
// `glossa approve` and `glossa import --history` need and an API token
// can never be.
//
// This file declares the flow's shape before both ends are built: the
// HTTP adapter and the CLI are written against it while the store, the
// codes and the bearer behind it are built in parallel.

// Device-flow errors. Each is a problem code of the same name
// (api/openapi.yaml, /v1/auth/device-sessions), so the CLI branches on
// RFC 8628 §3.5's vocabulary unchanged.
var (
	// ErrDeviceSignInUnavailable means this server does not run the
	// device flow.
	ErrDeviceSignInUnavailable = errors.New("identity: device sign-in is not available on this server")
	// ErrDeviceAuthorizationNotFound covers a user code that is unknown,
	// expired or already decided, without saying which.
	ErrDeviceAuthorizationNotFound = errors.New("identity: no pending device authorization with this code")
	ErrAuthorizationPending        = errors.New("identity: the person has not decided yet")
	ErrSlowDown                    = errors.New("identity: polling faster than the interval")
	ErrAccessDenied                = errors.New("identity: the person denied the device")
	ErrExpiredToken                = errors.New("identity: the device code is unknown, expired or spent")
	ErrInvalidClientName           = errors.New("identity: a device names itself in 1-100 characters")
)

// DeviceAuthorization is what the device shows and polls with.
type DeviceAuthorization struct {
	// DeviceCode is the device's polling secret, never shown.
	DeviceCode string
	// UserCode is shown to the person as XXXX-XXXX.
	UserCode string
	// VerificationURI is Studio's page for entering a code,
	// LinkBaseURL + "/device"; VerificationURIComplete adds
	// "?code=" + UserCode.
	VerificationURI         string
	VerificationURIComplete string
	ExpiresIn               time.Duration
	Interval                time.Duration
}

// DeviceAuthorizationView is what Studio shows before the person decides.
type DeviceAuthorizationView struct {
	UserCode    string
	ClientName  string
	RequestedAt time.Time
	ExpiresAt   time.Time
}

// DeviceSession is the bearer an approved device receives: the person's
// session, presented as `Authorization: Bearer glossa_dev_…`.
type DeviceSession struct {
	AccessToken string
	ExpiresAt   time.Time
	Person      domain.PersonID
}

// StartDeviceAuthorization issues a device code and a user code for a
// device that names itself.
func (s *Service) StartDeviceAuthorization(ctx context.Context, clientName string) (DeviceAuthorization, error) {
	return DeviceAuthorization{}, ErrDeviceSignInUnavailable
}

// DeviceAuthorizationFor looks a pending user code up for the signed-in
// person who is about to decide it.
func (s *Service) DeviceAuthorizationFor(ctx context.Context, person domain.PersonID, userCode string) (DeviceAuthorizationView, error) {
	return DeviceAuthorizationView{}, ErrDeviceSignInUnavailable
}

// DecideDeviceAuthorization approves (signing the device in as person)
// or denies a pending user code.
func (s *Service) DecideDeviceAuthorization(ctx context.Context, person domain.PersonID, userCode string, approve bool) error {
	return ErrDeviceSignInUnavailable
}

// RedeemDeviceAuthorization is the device's poll: ErrAuthorizationPending,
// ErrSlowDown, ErrAccessDenied or ErrExpiredToken until the code is
// approved, then the session, once.
func (s *Service) RedeemDeviceAuthorization(ctx context.Context, deviceCode string) (DeviceSession, error) {
	return DeviceSession{}, ErrDeviceSignInUnavailable
}
