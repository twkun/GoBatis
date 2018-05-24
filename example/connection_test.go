package example

import (
	"testing"

	"github.com/runner-mei/GoBatis/tests"
	"github.com/runner-mei/gobatis"
)

const (
	postgres = `
DROP TABLE IF EXISTS auth_users;

CREATE TABLE IF NOT EXISTS auth_users (
  id bigserial PRIMARY KEY,
  username VARCHAR(32) NOT NULL UNIQUE,
  Phone VARCHAR(32),
  address VARCHAR(256),
  status INT,
  birth_day DATE,
  created_at TIMESTAMP default NOW(),
  updated_at TIMESTAMP default NOW()
)`

	mysql = `
CREATE TABLE IF NOT EXISTS auth_users (
  id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(32) NOT NULL UNIQUE,
  Phone VARCHAR(32),
  address VARCHAR(256),
  status TINYINT UNSIGNED,
  birth_day DATE,
  created_at TIMESTAMP default CURRENT_TIMESTAMP,
  updated_at TIMESTAMP default CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8`
)

func TestConnection(t *testing.T) {
	insertUser := AuthUser{
		Username: "abc",
		Phone:    "123",
		Status:   1,
	}

	tests.Run(t, func(_ testing.TB, factory *gobatis.SessionFactory) {
		var err error
		switch factory.DbType() {
		case gobatis.DbTypePostgres:
			_, err = factory.DB().Exec(postgres)
		default:
			_, err = factory.DB().Exec(mysql)
		}
		if err != nil {
			t.Error(err)
			return
		}

		conn := NewConnection(factory)

		t.Run("insertAndGet", func(t *testing.T) {
			_, err := conn.Users().DeleteAll()
			if err != nil {
				t.Error(err)
				return
			}

			id, err := conn.Users().Insert(&insertUser)
			if err != nil {
				t.Error(err)
				return
			}

			u, err := conn.Users().Get(id)
			if err != nil {
				t.Error(err)
				return
			}

			if u.Username != insertUser.Username {
				t.Error("excepted is", u.Username, ", actual is", insertUser.Username)
			}

			if u.Phone != insertUser.Phone {
				t.Error("excepted is", u.Phone, ", actual is", insertUser.Phone)
			}

			name, err := conn.Users().GetNameByID(id)
			if err != nil {
				t.Error(err)
				return
			}

			if name != insertUser.Username {
				t.Error("excepted is", name, ", actual is", insertUser.Username)
			}

			count, err := conn.Users().Count()
			if err != nil {
				t.Error(err)
				return
			}

			if count != 1 {
				t.Error("excepted is", 1, ", actual is", count)
			}
		})

		t.Run("delete", func(t *testing.T) {
			_, err := conn.Users().DeleteAll()
			if err != nil {
				t.Error(err)
				return
			}

			id, err := conn.Users().Insert(&insertUser)
			if err != nil {
				t.Error(err)
				return
			}

			count, err := conn.Users().Delete(id)
			if err != nil {
				t.Error(err)
				return
			}

			if count != 1 {
				t.Error("excepted is", 1, ", actual is", count)
			}

			count, err = conn.Users().Count()
			if err != nil {
				t.Error(err)
				return
			}

			if count != 0 {
				t.Error("excepted is", 0, ", actual is", count)
			}
		})

		t.Run("update", func(t *testing.T) {
			_, err := conn.Users().DeleteAll()
			if err != nil {
				t.Error(err)
				return
			}

			id, err := conn.Users().Insert(&insertUser)
			if err != nil {
				t.Error(err)
				return
			}

			count, err := conn.Users().UpdateName(id, "newusername")
			if err != nil {
				t.Error(err)
				return
			}

			if count != 1 {
				t.Error("update rows is", count)
			}

			u, err := conn.Users().Get(id)
			if err != nil {
				t.Error(err)
				return
			}

			if u.Username == insertUser.Username {
				t.Error("excepted isnot newusername, actual is", u.Username)
			}

			if u.Username != "newusername" {
				t.Error("excepted is newusername, actual is", u.Username)
			}

			name, err := conn.Users().GetNameByID(id)
			if err != nil {
				t.Error(err)
				return
			}

			if name != "newusername" {
				t.Error("excepted is newusername, actual is", name)
			}
		})

	})
}
