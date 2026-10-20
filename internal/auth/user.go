package auth

import "time"

type User struct {
	ID        string
	Username  string
	Timezone  string
	CreatedAt time.Time
}

type CreateUserInput struct {
	Username     string
	PasswordHash string
	Timezone     string
}
