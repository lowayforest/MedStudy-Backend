package folder

import "time"

type Folder struct {
	ID        string
	UserID    string
	Name      string
	Type      string
	CreatedAt time.Time
}
