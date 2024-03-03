package models

import (
	"time"
)

type Profile struct {
	AccountId         string    `db:"account_id"`
	Username          string    `db:"username"`
	Email             string    `db:"email"`
	ProfilePictureUrl string    `db:"profile_picture_id"`
	RegistrationDate  time.Time `db:"registration_date"`
}

type RepositoryProfile struct {
	AccountId        string    `db:"account_id"`
	Username         string    `db:"username"`
	Email            string    `db:"email"`
	ProfilePictureId string    `db:"profile_picture_id"`
	RegistrationDate time.Time `db:"registration_date"`
}
