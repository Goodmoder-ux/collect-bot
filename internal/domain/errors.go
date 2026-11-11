package domain

import "errors"

// Sentinel errors that callers translate into user-facing messages.
var (
	// ErrCollectionNotActive is returned when an action targets a closed or missing collection.
	ErrCollectionNotActive = errors.New("collection is not active")
	// ErrAlreadyPaid is returned when a student already has a payment for the collection.
	ErrAlreadyPaid = errors.New("student already paid the collection")
	// ErrPaymentAlreadyMatched is returned when a payment is no longer waiting for manual review.
	ErrPaymentAlreadyMatched = errors.New("payment is already matched")
)
