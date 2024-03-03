package models

import (
	"time"
)

type Account struct {
	Id               string    `json:"id"`
	Email            string    `json:"email"`
	RegistrationDate time.Time `json:"registration_date"`
}
