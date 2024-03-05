package models

import (
	"time"
)

type Profile struct {
	AccountID         string
	Username          string
	Email             string
	ProfilePictureURL string
	RegistrationDate  time.Time
}

type RepositoryProfile struct {
	AccountID        string    `db:"account_id"`
	Username         string    `db:"username"`
	Email            string    `db:"email"`
	ProfilePictureID string    `db:"profile_picture_id"`
	RegistrationDate time.Time `db:"registration_date"`
}
