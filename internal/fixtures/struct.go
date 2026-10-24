package fixtures

import (
	"fmt"
	"time"
)

type TestStruct struct {
	ID             uint64
	Flags          uint64
	PrimaryEmail   string `validate:"required"`
	EmailSecondary string `validate:"required,email"`
	FirstName      string `validate:"required,min=2,max=30"`
	LastName       string `validate:"required,max=255"`
	Age            int    `validate:"min=1,max=120"`
	Price          int    `validate:"min=0,max=999"`
	PostCode       string `validate:"required,min=6" crud_regexp:"^[0-9]{2}\\-[0-9]{3}$"`
	PostCode2      string `validate:"min=6" crud_regexp:"^[0-9]{2}\\-[0-9]{3}$"`
	Password       string `json:"password"`
	Key            string `crud:"uniq" validate:"required,min=30,max=255"`
	CreatedAt      time.Time
	CreatedBy      uint64
	ModifiedAt     time.Time
	ModifiedBy     uint64
}

func TestStructWithData() *TestStruct {
	ts := &TestStruct{}
	ts.Flags = 4
	ts.PrimaryEmail = "primary@example.com"
	ts.EmailSecondary = "secondary@example.com"
	ts.FirstName = "John"
	ts.LastName = "Smith"
	ts.Age = 37
	ts.Price = 444
	ts.PostCode = "00-000"
	ts.PostCode2 = "11-111"
	ts.Password = "yyy"
	ts.Key = fmt.Sprintf("12345679012345678901234567890%d", time.Now().UnixNano())
	return ts
}

func AreTestStructObjectsSame(ts1 *TestStruct, ts2 *TestStruct) bool {
	if ts1.Flags != ts2.Flags {
		return false
	}
	if ts1.PrimaryEmail != ts2.PrimaryEmail {
		return false
	}
	if ts1.EmailSecondary != ts2.EmailSecondary {
		return false
	}
	if ts1.FirstName != ts2.FirstName {
		return false
	}
	if ts1.LastName != ts2.LastName {
		return false
	}
	if ts1.Age != ts2.Age {
		return false
	}
	if ts1.Price != ts2.Price {
		return false
	}
	if ts1.PostCode != ts2.PostCode {
		return false
	}
	if ts1.PostCode2 != ts2.PostCode2 {
		return false
	}
	if ts1.Password != ts2.Password {
		return false
	}
	if ts1.CreatedBy != ts2.CreatedBy {
		return false
	}
	if ts1.Key != ts2.Key {
		return false
	}
	return true
}
