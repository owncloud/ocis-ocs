package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"github.com/owncloud/ocis-ocs/pkg/config"
	svc "github.com/owncloud/ocis-ocs/pkg/service/v0"
	ocisLog "github.com/owncloud/ocis-pkg/v2/log"
	"github.com/stretchr/testify/assert"

	"github.com/owncloud/ocis-pkg/v2/service/grpc"

	accountsCmd "github.com/owncloud/ocis-accounts/pkg/command"
	accountsCfg "github.com/owncloud/ocis-accounts/pkg/config"
	accountsProto "github.com/owncloud/ocis-accounts/pkg/proto/v0"
	accountsSvc "github.com/owncloud/ocis-accounts/pkg/service/v0"
)

var service = grpc.Service{}

var formats = []string{"json", "xml"}

var DefaultUsers = []string{
	"4c510ada-c86b-4815-8820-42cdf82c3d51",
	"820ba2a1-3f54-4538-80a4-2d73007e30bf",
	"932b4540-8d16-481e-8ef4-588e4b6b151c",
	"bc596f3c-c955-4328-80a0-60d018b4ad57",
	"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
}

func getFormatString(format string) string {
	if format == "json" {
		return "?format=json"
	} else if format == "xml" {
		return ""
	} else {
		panic("Invalid format received")
	}
}

type User struct {
	Enabled     string `json:"enabled" xml:"enabled"`
	ID          string `json:"id" xml:"id"`
	Username    string `json:"username" xml:"username"`
	Email       string `json:"email" xml:"email"`
	Quota       int    `json:"quota" xml:"quota"`
	UIDNumber   int    `json:"uidnumber" xml:"uidnumber"`
	GIDNumber   int    `json:"gidnumber" xml:"gidnumber"`
	Displayname string `json:"displayname" xml:"displayname"`
}

func (u *User) getUserRequestString() string {
	res := fmt.Sprintf("userid=%v&username=%v&email=%v", u.ID, u.Username, u.Email)

	if u.Displayname != "" {
		res = res + "&displayname=" + fmt.Sprint(u.Displayname)
	}

	if u.UIDNumber != 0 {
		res = res + "&uidnumber=" + fmt.Sprint(u.UIDNumber)
	}

	if u.GIDNumber != 0 {
		res = res + "&gidnumber=" + fmt.Sprint(u.GIDNumber)
	}

	return res
}

type Meta struct {
	Status     string `json:"status" xml:"status"`
	StatusCode int    `json:"statuscode" xml:"statuscode"`
	Message    string `json:"message" xml:"message"`
}

func (m *Meta) Success() bool {
	if m.Status == "ok" &&
		(m.StatusCode >= 100 && m.StatusCode <= 300) {
		return true
	}
	return false
}

type SingleUserResponse struct {
	Meta Meta `json:"meta" xml:"meta"`
	Data User `json:"data" xml:"data"`
}

type GetUsersResponse struct {
	Meta Meta `json:"meta" xml:"meta"`
	Data struct {
		Users []string `json:"users" xml:"users>element"`
	} `json:"data" xml:"data"`
}

type DeleteUserRespone struct {
	Meta Meta `json:"meta" xml:"meta"`
	Data struct {
	} `json:"data" xml:"data"`
}

func assertResponseMeta(t *testing.T, expected, actual Meta) {
	assert.Equal(t, expected.Status, actual.Status, "The status of response doesn't matches")
	assert.Equal(t, expected.StatusCode, actual.StatusCode, "The Status code of response doesn't matches")
	assert.Equal(t, expected.Message, actual.Message, "The Message of response doesn't matches")
}

//func assertUserSame(t *testing.T, expected, actual User) {
//assert.Equal(t, expected.ID, actual.ID, "UserId doesn't match for user %v", expected.Username)
//assert.Equal(t, expected.Username, actual.Username, "Username doesn't match for user %v", expected.Username)
//assert.Equal(t, expected.Email, actual.Email, "email doesn't match for user %v", expected.Username)

//assert.Equal(t, expected.Enabled, actual.Enabled, "enabled doesn't match for user %v", expected.Username)
//assert.Equal(t, expected.Quota, actual.Quota, "Quota match for user %v", expected.Username)
//assert.Equal(t, expected.UIDNumber, actual.UIDNumber, "UidNumber doesn't match for user %v", expected.Username)
//assert.Equal(t, expected.GIDNumber, actual.GIDNumber, "GIDNumber doesn't match for user %v", expected.Username)
//assert.Equal(t, expected.Displayname, actual.Displayname, "displayname doesn't match for user %v", expected.Username)
//}

const dataPath = "./accounts-store"

func deleteAccount(t *testing.T, id string) (*empty.Empty, error) {
	client := service.Client()
	cl := accountsProto.NewAccountsService("com.owncloud.api.accounts", client)

	req := &accountsProto.DeleteAccountRequest{Id: id}
	res, err := cl.DeleteAccount(context.Background(), req)
	return res, err
}

func init() {
	service = grpc.NewService(
		grpc.Namespace("com.owncloud.api"),
		grpc.Name("accounts"),
		grpc.Address("localhost:9180"),
	)

	cfg := accountsCfg.New()
	cfg.Server.AccountsDataPath = dataPath
	var hdlr *accountsSvc.Service
	var err error

	if hdlr, err = accountsSvc.New(
		accountsSvc.Logger(accountsCmd.NewLogger(cfg)),
		accountsSvc.Config(cfg)); err != nil {
		log.Fatalf("Could not create new service")
	}

	err = accountsProto.RegisterAccountsServiceHandler(service.Server(), hdlr)
	if err != nil {
		log.Fatal("could not register the Accounts handler")
	}
	err = accountsProto.RegisterGroupsServiceHandler(service.Server(), hdlr)
	if err != nil {
		log.Fatal("could not register the Groups handler")
	}

	err = service.Server().Start()
	if err != nil {
		log.Fatal(err)
	}
}

func cleanUp(t *testing.T) {
	datastore := filepath.Join(dataPath, "accounts")

	files, err := ioutil.ReadDir(datastore)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		found := false
		for _, defUser := range DefaultUsers {
			if f.Name() == defUser {
				found = true
				break
			}
		}

		if !found {
			deleteAccount(t, f.Name())
		}
	}
}

func sendRequest(method, endpoint, body, auth string) (*httptest.ResponseRecorder, error) {
	var reader = strings.NewReader(body)
	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if auth != "" {
		req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(auth)))
	}

	rr := httptest.NewRecorder()

	service := getService()
	service.ServeHTTP(rr, req)

	return rr, nil
}

func getService() svc.Service {
	c := &config.Config{
		HTTP: config.HTTP{
			Root:      "/",
			Addr:      "localhost:9110",
			Namespace: "com.owncloud.web",
		},
		TokenManager: config.TokenManager{
			JWTSecret: "HELLO-secret",
		},
	}

	var logger ocisLog.Logger

	svc := svc.NewService(
		svc.Logger(logger),
		svc.Config(c),
	)

	return svc
}

func createUser(u User) error {
	_, err := sendRequest(
		"POST",
		"/v1.php/cloud/users?format=json",
		u.getUserRequestString(),
		"admin:admin",
	)

	if err != nil {
		return err
	}
	return nil
}

func TestCreateUser(t *testing.T) {
	testData := []struct {
		user User
		err  *Meta
	}{
		// A simple user
		{
			User{
				Enabled:     "true",
				Username:    "rutherford",
				ID:          "rutherford",
				Email:       "rutherford@example.com",
				Displayname: "Ernest RutherFord",
			},
			nil,
		},

		// User with Uid and Gid defined
		{
			User{
				Enabled:     "true",
				Username:    "thomson",
				ID:          "thomson",
				Email:       "thomson@example.com",
				Displayname: "J. J. Thomson",
				UIDNumber:   20027,
				GIDNumber:   30000,
			},
			nil,
		},

		// User with different username and Id
		{
			User{
				Enabled:     "true",
				Username:    "niels",
				ID:          "bohr",
				Email:       "bohr@example.com",
				Displayname: "Niels Bohr",
			},
			nil,
		},

		// User with special character in username
		{
			User{
				Enabled:     "true",
				Username:    "schrödinger",
				ID:          "schrödinger",
				Email:       "schrödinger@example.com",
				Displayname: "Erwin Schrödinger",
			},
			&Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "preferred_name 'schrödinger' must be at least the local part of an email",
			},
		},
	}

	for _, format := range formats {
		for _, data := range testData {
			formatpart := getFormatString(format)
			res, err := sendRequest(
				"POST",
				"/v1.php/cloud/users"+formatpart,
				data.user.getUserRequestString(),
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var response SingleUserResponse

			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			}

			if data.err == nil {
				assert.True(t, response.Meta.Success(), "The response was expected to be successful but was not")
				// assertUserSame(t, data.user, response.Data)
			} else {
				assertResponseMeta(t, *data.err, response.Meta)
			}
		}
	}

	cleanUp(t)
}

func TestGetUsers(t *testing.T) {
	users := []User{
		{
			Enabled:     "true",
			Username:    "rutherford",
			ID:          "rutherford",
			Email:       "rutherford@example.com",
			Displayname: "Ernest RutherFord",
		},
		{
			Enabled:     "true",
			Username:    "thomson",
			ID:          "thomson",
			Email:       "thomson@example.com",
			Displayname: "J. J. Thomson",
		},
	}

	for _, format := range formats {
		for _, user := range users {
			err := createUser(user)
			if err != nil {
				t.Fatal(err)
			}
		}

		formatpart := getFormatString(format)
		res, err := sendRequest(
			"GET",
			"/v1.php/cloud/users"+formatpart,
			"",
			"admin:admin",
		)

		if err != nil {
			t.Fatal(err)
		}

		var response GetUsersResponse

		if format == "json" {
			if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}

		assert.True(t, response.Meta.Success(), "The response was expected to be successful but was not")
		for _, user := range users {
			assert.Contains(t, response.Data.Users, user.Username)
		}
		cleanUp(t)
	}
}

func TestGetUsersDefaultUsers(t *testing.T) {
	for _, format := range formats {
		formatpart := getFormatString(format)
		res, err := sendRequest(
			"GET",
			"/v1.php/cloud/users"+formatpart,
			"",
			"admin:admin",
		)

		if err != nil {
			t.Fatal(err)
		}

		var response GetUsersResponse
		if format == "json" {
			if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}

		assert.True(t, response.Meta.Success(), "The response was expected to be successful but was not")
		for _, user := range DefaultUsers {
			assert.Contains(t, response.Data.Users, user)
		}
		cleanUp(t)
	}
}

func TestGetUserDefaultUser(t *testing.T) {
	for _, format := range formats {
		formatpart := getFormatString(format)
		for _, user := range DefaultUsers {
			res, err := sendRequest(
				"GET",
				fmt.Sprintf("/v1.php/cloud/user/%s", user)+formatpart,
				"",
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var response SingleUserResponse
			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			}

			assertResponseMeta(t, Meta{
				Status:     "error",
				StatusCode: 998,
				Message:    "not found",
			}, response.Meta)
			cleanUp(t)
		}
	}
}

func TestDeleteUser(t *testing.T) {
	time.Sleep(time.Second * 2)
	users := []User{
		{
			Enabled:     "true",
			Username:    "rutherford",
			ID:          "rutherford",
			Email:       "rutherford@example.com",
			Displayname: "Ernest RutherFord",
		},
		{
			Enabled:     "true",
			Username:    "thomson",
			ID:          "thomson",
			Email:       "thomson@example.com",
			Displayname: "J. J. Thomson",
		},
	}

	for _, format := range formats {
		for _, user := range users {
			err := createUser(user)
			if err != nil {
				t.Fatal(err)
			}
		}

		formatpart := getFormatString(format)
		res, err := sendRequest(
			"DELETE",
			"/v1.php/cloud/users/rutherford"+formatpart,
			"",
			"admin:admin",
		)

		if err != nil {
			t.Fatal(err)
		}

		var response DeleteUserRespone
		if format == "json" {
			if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}

		assert.True(t, response.Meta.Success(), "The response was expected to be successful but was not")
		assert.Empty(t, response.Data)

		// Check deleted user doesn't exist and the other user does
		res, err = sendRequest(
			"GET",
			"/v1.php/cloud/users?format=json",
			"",
			"admin:admin",
		)

		if err != nil {
			t.Fatal(err)
		}

		var usersResponse GetUsersResponse
		if err := json.Unmarshal(res.Body.Bytes(), &usersResponse); err != nil {
			t.Fatal(err)
		}

		assert.True(t, usersResponse.Meta.Success(), "The response was expected to be successful but was not")
		assert.Contains(t, usersResponse.Data.Users, "thomson")
		assert.NotContains(t, usersResponse.Data.Users, "rutherford")

		cleanUp(t)
	}
}

func TestUpdateUser(t *testing.T) {
	time.Sleep(time.Second * 2)
	users := []User{
		{
			Enabled:     "true",
			Username:    "rutherford",
			ID:          "rutherford",
			Email:       "rutherford@example.com",
			Displayname: "Ernest RutherFord",
		},
		{
			Enabled:     "true",
			Username:    "thomson",
			ID:          "thomson",
			Email:       "thomson@example.com",
			Displayname: "J. J. Thomson",
		},
	}

	for _, format := range formats {
		formatpart := getFormatString(format)
		for _, user := range users {
			err := createUser(user)
			if err != nil {
				t.Fatal(err)
			}
		}

		res, err := sendRequest(
			"PUT",
			"/v1.php/cloud/users/rutherford"+formatpart,
			"username=chadwick&displayname=James%20Chadwick",
			"admin:admin",
		)

		if err != nil {
			t.Fatal(err)
		}

		var response struct {
			Meta Meta `json:"meta" xml:"meta"`
		}

		if format == "json" {
			if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
		}

		assertResponseMeta(t, Meta{
			Status:     "error",
			StatusCode: 103,
			Message:    "unknown key ''",
		}, response.Meta)

		cleanUp(t)
	}
}
