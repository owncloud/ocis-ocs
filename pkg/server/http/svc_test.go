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
	"net/url"
	"path/filepath"
	"strings"
	"testing"

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

	"github.com/micro/go-micro/v2/client"
	settings "github.com/owncloud/ocis-settings/pkg/proto/v0"
)

var service = grpc.Service{}

var mockedRoleAssignment = map[string]string{}

var ocsVersions = []string{"v1.php", "v2.php"}

var formats = []string{"json", "xml"}

const dataPath = "./accounts-store"

var DefaultUsers = []string{
	"4c510ada-c86b-4815-8820-42cdf82c3d51",
	"820ba2a1-3f54-4538-80a4-2d73007e30bf",
	"932b4540-8d16-481e-8ef4-588e4b6b151c",
	"bc596f3c-c955-4328-80a0-60d018b4ad57",
	"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c",
	"058bff95-6708-4fe5-91e4-9ea3d377588b",
}

var DefaultGroups = []string{
	"167cbee2-0518-455a-bfb2-031fe0621e5d",
	"262982c1-2362-4afa-bfdf-8cbfef64a06e",
	"34f38767-c937-4eb6-b847-1c175829a2a0",
	"509a9dcd-bb37-4f4f-a01a-19dca27d9cfa",
	"6040aa17-9c64-4fef-9bd0-77234d71bad0",
	"7b87fd49-286e-4a5f-bafd-c535d5dd997a",
	"a1726108-01f8-4c30-88df-2b1a9d1cba1a",
	"cedc21aa-4072-4614-8676-fa9165f598ff",
	"dd58e5ec-842e-498b-8800-61f2ec6f911f",
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

type Quota struct {
	Free       int64   `json:"free" xml:"free"`
	Used       int64   `json:"used" xml:"used"`
	Total      int64   `json:"total" xml:"total"`
	Relative   float32 `json:"relative" xml:"relative"`
	Definition string  `json:"definition" xml:"definition"`
}

type User struct {
	Enabled     string `json:"enabled" xml:"enabled"`
	ID          string `json:"id" xml:"id"`
	Username    string `json:"username" xml:"username"`
	Email       string `json:"email" xml:"email"`
	Password    string `json:"-" xml:"-"`
	Quota       Quota  `json:"quota" xml:"quota"`
	UIDNumber   int    `json:"uidnumber" xml:"uidnumber"`
	GIDNumber   int    `json:"gidnumber" xml:"gidnumber"`
	Displayname string `json:"displayname" xml:"displayname"`
}

type Group struct {
	ID          string `json:"id" xml:"id"`
	GIDNumber   int    `json:"gidnumber" xml:"gidnumber"`
	Displayname string `json:"displayname" xml:"displayname"`
}

func (u *User) getUserRequestString() string {
	res := url.Values{}

	if u.Password != "" {
		res.Add("password", u.Password)
	}

	if u.ID != "" {
		res.Add("userid", u.ID)
	}

	if u.Username != "" {
		res.Add("username", u.Username)
	}

	if u.Email != "" {
		res.Add("email", u.Email)
	}

	if u.Displayname != "" {
		res.Add("displayname", u.Displayname)
	}

	if u.UIDNumber != 0 {
		res.Add("uidnumber", fmt.Sprint(u.UIDNumber))
	}

	if u.GIDNumber != 0 {
		res.Add("gidnumber", fmt.Sprint(u.GIDNumber))
	}

	return res.Encode()
}

func (g *Group) getGroupRequestString() string {
	res := url.Values{}

	if g.ID != "" {
		res.Add("groupid", g.ID)
	}

	if g.Displayname != "" {
		res.Add("displayname", g.Displayname)
	}
	if g.GIDNumber != 0 {
		res.Add("gidnumber", fmt.Sprint(g.GIDNumber))
	}

	return res.Encode()
}

type Meta struct {
	Status     string `json:"status" xml:"status"`
	StatusCode int    `json:"statuscode" xml:"statuscode"`
	Message    string `json:"message" xml:"message"`
}

func (m *Meta) Success(ocsVersion string) bool {
	if !(ocsVersion == "v1.php" || ocsVersion == "v2.php") {
		return false
	}
	if m.Status != "ok" {
		return false
	}
	if ocsVersion == "v1.php" && m.StatusCode != 100 {
		return false
	} else if ocsVersion == "v2.php" && m.StatusCode != 200 {
		return false
	} else {
		return true
	}
}

type SingleUserResponse struct {
	Ocs struct {
		Meta Meta `json:"meta" xml:"meta"`
		Data User `json:"data" xml:"data"`
	} `json:"ocs" xml:"ocs"`
}

type GetUsersResponse struct {
	Ocs struct {
		Meta Meta `json:"meta" xml:"meta"`
		Data struct {
			Users []string `json:"users" xml:"users>element"`
		} `json:"data" xml:"data"`
	} `json:"ocs" xml:"ocs"`
}

type EmptyResponse struct {
	Ocs struct {
		Meta Meta `json:"meta" xml:"meta"`
		Data struct {
		} `json:"data" xml:"data"`
	} `json:"ocs" xml:"ocs"`
}

type GetUsersGroupsResponse struct {
	Ocs struct {
		Meta Meta `json:"meta" xml:"meta"`
		Data struct {
			Groups []string `json:"groups" xml:"groups>element"`
		} `json:"data" xml:"data"`
	} `json:"ocs" xml:"ocs"`
}

type GetGroupsResponse struct {
	Ocs struct {
		Meta Meta `json:"meta" xml:"meta"`
		Data struct {
			Groups []string `json:"groups" xml:"groups>element"`
		} `json:"data" xml:"data"`
	} `json:"ocs" xml:"ocs"`
}

type GetGroupMembersResponse struct {
	Ocs struct {
		Meta Meta `json:"meta" xml:"meta"`
		Data struct {
			Users []string `json:"users" xml:"users>element"`
		} `json:"data" xml:"data"`
	} `json:"ocs" xml:"ocs"`
}

func assertStatusCode(t *testing.T, statusCode int, res *httptest.ResponseRecorder, ocsVersion string) {
	if ocsVersion == "v1.php" {
		assert.Equal(t, 200, res.Code)
	} else {
		assert.Equal(t, statusCode, res.Code)
	}
}

func assertResponseMeta(t *testing.T, expected, actual Meta) {
	assert.Equal(t, expected.Status, actual.Status, "The status of response doesn't matches")
	assert.Equal(t, expected.StatusCode, actual.StatusCode, "The Status code of response doesn't matches")
	assert.Equal(t, expected.Message, actual.Message, "The Message of response doesn't matches")
}

func assertUserSame(t *testing.T, expected, actual User, quotaAvailable bool) {
	if expected.ID == "" {
		// Check the auto generated userId
		assert.Regexp(
			t,
			"[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}",
			actual.ID, "the userid is not a valid uuid",
		)
	} else {
		assert.Equal(t, expected.ID, actual.ID, "UserId doesn't match for user %v", expected.Username)
	}
	assert.Equal(t, expected.Username, actual.Username, "Username doesn't match for user %v", expected.Username)
	assert.Equal(t, expected.Email, actual.Email, "email doesn't match for user %v", expected.Username)
	assert.Equal(t, expected.Enabled, actual.Enabled, "enabled doesn't match for user %v", expected.Username)
	assert.Equal(t, expected.Displayname, actual.Displayname, "displayname doesn't match for user %v", expected.Username)
	if quotaAvailable {
		assert.NotZero(t, actual.Quota.Free)
		assert.NotZero(t, actual.Quota.Used)
		assert.NotZero(t, actual.Quota.Total)
		assert.Equal(t, "default", actual.Quota.Definition)
	} else {
		assert.Equal(t, expected.Quota, actual.Quota, "Quota match for user %v", expected.Username)
	}

	// FIXME: gidnumber and Uidnumber are always 0
	// https://github.com/owncloud/ocis-ocs/issues/45
	assert.Equal(t, 0, actual.UIDNumber, "UidNumber doesn't match for user %v", expected.Username)
	assert.Equal(t, 0, actual.GIDNumber, "GIDNumber doesn't match for user %v", expected.Username)

}

func deleteAccount(t *testing.T, id string) (*empty.Empty, error) {
	client := service.Client()
	cl := accountsProto.NewAccountsService("com.owncloud.api.accounts", client)

	req := &accountsProto.DeleteAccountRequest{Id: id}
	res, err := cl.DeleteAccount(context.Background(), req)
	return res, err
}

func deleteGroup(t *testing.T, id string) (*empty.Empty, error) {
	client := service.Client()
	cl := accountsProto.NewGroupsService("com.owncloud.api.accounts", client)

	req := &accountsProto.DeleteGroupRequest{Id: id}
	res, err := cl.DeleteGroup(context.Background(), req)
	return res, err
}

func buildRoleServiceMock() settings.RoleService {
	return settings.MockRoleService{
		AssignRoleToUserFunc: func(ctx context.Context, req *settings.AssignRoleToUserRequest, opts ...client.CallOption) (res *settings.AssignRoleToUserResponse, err error) {
			mockedRoleAssignment[req.AccountUuid] = req.RoleId
			return &settings.AssignRoleToUserResponse{
				Assignment: &settings.UserRoleAssignment{
					AccountUuid: req.AccountUuid,
					RoleId:      req.RoleId,
				},
			}, nil
		},
	}
}

func init() {
	service = grpc.NewService(
		grpc.Namespace("com.owncloud.api"),
		grpc.Name("accounts"),
		grpc.Address("localhost:9180"),
	)

	c := &accountsCfg.Config{
		Server: accountsCfg.Server{
			AccountsDataPath: dataPath,
		},
		Log: accountsCfg.Log{
			Level:  "info",
			Pretty: true,
			Color:  true,
		},
	}

	var hdlr *accountsSvc.Service
	var err error

	if hdlr, err = accountsSvc.New(
		accountsSvc.Logger(accountsCmd.NewLogger(c)),
		accountsSvc.Config(c),
		accountsSvc.RoleService(buildRoleServiceMock())); err != nil {
		log.Fatalf("Could not create new service")
	}

	hdlr.Client = mockClient{}

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
		log.Fatalf("could not start server: %v", err)
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

	datastoreGroups := filepath.Join(dataPath, "groups")

	files, err = ioutil.ReadDir(datastoreGroups)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
		found := false
		for _, defGrp := range DefaultGroups {
			if f.Name() == defGrp {
				found = true
				break
			}
		}

		if !found {
			deleteGroup(t, f.Name())
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
		Log: config.Log{
			Level: "debug",
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

func createGroup(g Group) error { //lint:file-ignore U1000 not implemented
	_, err := sendRequest(
		"POST",
		"/v1.php/cloud/users?format=json",
		g.getGroupRequestString(),
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
				Displayname: "ErnestRutherFord",
				Password:    "newPassword",
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
				Password:    "newPassword",
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
				Password:    "newPassword",
			},
			nil,
		},

		// User withoutl password
		// https://github.com/owncloud/ocis-ocs/issues/50
		{
			User{
				Enabled:     "true",
				Username:    "john",
				ID:          "john",
				Email:       "john@example.com",
				Displayname: "John Dalton",
			},
			nil,
		},

		// User with special character in username
		// https://github.com/owncloud/ocis-ocs/issues/49
		{
			User{
				Enabled:     "true",
				Username:    "schrödinger",
				ID:          "schrödinger",
				Email:       "schrödinger@example.com",
				Displayname: "Erwin Schrödinger",
				Password:    "newPassword",
			},
			&Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "preferred_name 'schrödinger' must be at least the local part of an email",
			},
		},

		// User with different userid and email
		{
			User{
				Enabled:     "true",
				Username:    "planck",
				ID:          "planck",
				Email:       "max@example.com",
				Displayname: "Max Planck",
				Password:    "newPassword",
			},
			nil,
		},

		// User with different userid and email and username
		{
			User{
				Enabled:     "true",
				Username:    "hisenberg",
				ID:          "hberg",
				Email:       "werner@example.com",
				Displayname: "Werner Hisenberg",
				Password:    "newPassword",
			},
			nil,
		},

		// User without displayname
		{
			User{
				Enabled:  "true",
				Username: "oppenheimer",
				ID:       "oppenheimer",
				Email:    "robert@example.com",
				Password: "newPassword",
			},
			nil,
		},

		// User wit invalid email
		{
			User{
				Enabled:  "true",
				Username: "chadwick",
				ID:       "chadwick",
				Email:    "not_a_email",
				Password: "newPassword",
			},
			&Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "mail 'not_a_email' must be a valid email",
			},
		},

		// User without email
		{
			User{
				Enabled:  "true",
				Username: "chadwick",
				ID:       "chadwick",
				Password: "newPassword",
			},
			&Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "mail '' must be a valid email",
			},
		},

		// User without username
		{
			User{
				Enabled:  "true",
				ID:       "chadwick",
				Email:    "james@example.com",
				Password: "newPassword",
			},
			&Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "preferred_name '' must be at least the local part of an email",
			},
		},

		// User without userid
		{
			User{
				Enabled:  "true",
				Username: "chadwick",
				Email:    "james@example.com",
				Password: "newPassword",
			},
			nil,
		},
	}

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			for _, data := range testData {
				formatpart := getFormatString(format)
				res, err := sendRequest(
					"POST",
					fmt.Sprintf("/%v/cloud/users%v", ocsVersion, formatpart),
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
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				if data.err == nil {
					assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
					assertStatusCode(t, 200, res, ocsVersion)
					assertUserSame(t, data.user, response.Ocs.Data, false)
				} else {
					assertStatusCode(t, 400, res, ocsVersion)
					assertResponseMeta(t, *data.err, response.Ocs.Meta)
				}

				var id string
				if data.user.ID != "" {
					id = data.user.ID
				} else {
					id = response.Ocs.Data.ID
				}

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

				assert.True(t, usersResponse.Ocs.Meta.Success("v1.php"), "The response was expected to be successful but was not")

				if data.err == nil {
					assert.Contains(t, usersResponse.Ocs.Data.Users, id)
				} else {
					assert.NotContains(t, usersResponse.Ocs.Data.Users, data.user.ID)
				}
			}
			cleanUp(t)
		}
	}
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

	for _, ocsVersion := range ocsVersions {
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
				fmt.Sprintf("/%v/cloud/users%v", ocsVersion, formatpart),
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
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 200, res, ocsVersion)
			assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
			for _, user := range users {
				assert.Contains(t, response.Ocs.Data.Users, user.Username)
			}
			cleanUp(t)
		}
	}
}

func TestGetUsersDefaultUsers(t *testing.T) {
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			res, err := sendRequest(
				"GET",
				fmt.Sprintf("/%v/cloud/users%v", ocsVersion, formatpart),
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
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 200, res, ocsVersion)
			assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
			for _, user := range DefaultUsers {
				assert.Contains(t, response.Ocs.Data.Users, user)
			}
			cleanUp(t)
		}
	}
}

func TestGetUser(t *testing.T) {
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

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {

			for _, user := range users {
				err := createUser(user)
				if err != nil {
					t.Fatal(err)
				}
			}
			formatpart := getFormatString(format)
			for _, user := range users {
				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%s/cloud/users/%s%s", ocsVersion, user.ID, formatpart),
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
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 200, res, ocsVersion)
				assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to pass but it failed")
				assertUserSame(t, user, response.Ocs.Data, true)
			}
			cleanUp(t)
		}
	}
}

func TestGetUserInvalidId(t *testing.T) {
	invalidUsers := []string{
		"1",
		"invalid",
		"3434234233",
		"1am41validUs3r",
		"_-@@--$$__",
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, user := range invalidUsers {
				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%s/cloud/user/%s%s", ocsVersion, user, formatpart),
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
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 404, res, ocsVersion)
				assert.False(t, response.Ocs.Meta.Success(ocsVersion), "the response was expected to fail but passed")
				assertResponseMeta(t, Meta{
					Status:     "error",
					StatusCode: 998,
					Message:    "not found",
				}, response.Ocs.Meta)
				cleanUp(t)
			}
		}
	}
}
func TestDeleteUser(t *testing.T) {
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

	for _, ocsVersion := range ocsVersions {
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
				fmt.Sprintf("/%s/cloud/users/rutherford%s", ocsVersion, formatpart),
				"",
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var response EmptyResponse
			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 200, res, ocsVersion)
			assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
			assert.Empty(t, response.Ocs.Data)

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

			assert.True(t, usersResponse.Ocs.Meta.Success("v1.php"), "The response was expected to be successful but was not")
			assert.Contains(t, usersResponse.Ocs.Data.Users, "thomson")
			assert.NotContains(t, usersResponse.Ocs.Data.Users, "rutherford")

			cleanUp(t)
		}
	}
}

func TestDeleteUserInvalidId(t *testing.T) {

	invalidUsers := []string{
		"1",
		"invalid",
		"3434234233",
		"1am41validUs3r",
		"_-@@--$$__",
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			for _, user := range invalidUsers {
				formatpart := getFormatString(format)
				res, err := sendRequest(
					"DELETE",
					fmt.Sprintf("/%s/cloud/users/%s%s", ocsVersion, user, formatpart),
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 404, res, ocsVersion)
				assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was not expected to be successful but was")
				assert.Empty(t, response.Ocs.Data)

				assertResponseMeta(t, Meta{
					Status:     "error",
					StatusCode: 998,
					Message:    "The requested user could not be found",
				}, response.Ocs.Meta)
			}
		}
	}
}

func TestUpdateUser(t *testing.T) {
	user := User{
		Enabled:     "true",
		Username:    "rutherford",
		ID:          "rutherford",
		Email:       "rutherford@example.com",
		Displayname: "Ernest RutherFord",
	}

	testData := []struct {
		UpdateKey   string
		UpdateValue string
		Error       *Meta
	}{
		{
			"displayname",
			"James Chadwick",
			nil,
		},
		{
			"display",
			"Neils Bohr",
			nil,
		},
		{
			"email",
			"ford@user.org",
			nil,
		},
		// Invalid email doesn't gives error
		// https://github.com/owncloud/ocis-ocs/issues/46
		{
			"email",
			"not_a_valid_email",
			nil,
		},
		{
			"password",
			"strongpass1234",
			nil,
		},
		{
			"username",
			"e_rutherford",
			nil,
		},
		// Empty values doesn't gives error
		// https://github.com/owncloud/ocis-ocs/issues/51
		{
			"email",
			"",
			nil,
		},
		{
			"username",
			"",
			nil,
		},
		{
			"password",
			"",
			nil,
		},

		// Invalid Keys
		{
			"invalid_key",
			"validvalue",
			&Meta{
				Status:     "error",
				StatusCode: 103,
				Message:    "unknown key 'invalid_key'",
			},
		},
		{
			"12345",
			"validvalue",
			&Meta{
				Status:     "error",
				StatusCode: 103,
				Message:    "unknown key '12345'",
			},
		},
		{
			"",
			"validvalue",
			&Meta{
				Status:     "error",
				StatusCode: 103,
				Message:    "unknown key ''",
			},
		},
		{
			"",
			"",
			&Meta{
				Status:     "error",
				StatusCode: 103,
				Message:    "unknown key ''",
			},
		},
	}

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, data := range testData {
				err := createUser(user)
				if err != nil {
					t.Fatalf("Failed while creating user: %v", err)
				}

				params := url.Values{}

				params.Add("key", data.UpdateKey)
				params.Add("value", data.UpdateValue)

				res, err := sendRequest(
					"PUT",
					fmt.Sprintf("/%s/cloud/users/rutherford%s", ocsVersion, formatpart),
					params.Encode(),
					"admin:admin",
				)

				updatedUser := user
				switch data.UpdateKey {
				case "username":
					updatedUser.Username = data.UpdateValue
				case "email":
					updatedUser.Email = data.UpdateValue
				case "displayname":
					updatedUser.Displayname = data.UpdateValue
				case "display":
					updatedUser.Displayname = data.UpdateValue
				}

				if err != nil {
					t.Fatal(err)
				}

				var response struct {
					Ocs struct {
						Meta Meta `json:"meta" xml:"meta"`
					} `json:"ocs" xml:"ocs"`
				}

				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				if data.Error != nil {
					assertResponseMeta(t, *data.Error, response.Ocs.Meta)
					assertStatusCode(t, 400, res, ocsVersion)
				} else {
					assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but failed")
					assertStatusCode(t, 200, res, ocsVersion)
				}

				// Check deleted user doesn't exist and the other user does
				res, err = sendRequest(
					"GET",
					"/v1.php/cloud/users/rutherford?format=json",
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var usersResponse SingleUserResponse
				if err := json.Unmarshal(res.Body.Bytes(), &usersResponse); err != nil {
					t.Fatal(err)
				}

				assert.True(t, usersResponse.Ocs.Meta.Success("v1.php"), "The response was expected to be successful but was not")
				if data.Error == nil {
					assertUserSame(t, updatedUser, usersResponse.Ocs.Data, true)
				} else {
					assertUserSame(t, user, usersResponse.Ocs.Data, true)
				}
				cleanUp(t)
			}
		}
	}
}

// This is a bug demonstration test for endpoint '/cloud/user'
// Link to the issue: https://github.com/owncloud/ocis-ocs/issues/52
func TestGetSingleUser(t *testing.T) {
	user := User{
		Enabled:     "true",
		Username:    "rutherford",
		ID:          "rutherford",
		Email:       "rutherford@example.com",
		Displayname: "Ernest RutherFord",
		Password:    "password",
	}

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			err := createUser(user)
			if err != nil {
				t.Fatal(err)
			}

			formatpart := getFormatString(format)
			res, err := sendRequest(
				"GET",
				fmt.Sprintf("/%v/cloud/user%v", ocsVersion, formatpart),
				"",
				fmt.Sprintf("%v:%v", user.Username, user.Password),
			)

			if err != nil {
				t.Fatal(err)
			}

			var response EmptyResponse

			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					log.Println(err)
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 400, res, ocsVersion)
			assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be a failure but was not")
			assertResponseMeta(t, Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "missing user in context",
			}, response.Ocs.Meta)
			assert.Empty(t, response.Ocs.Data)
			cleanUp(t)
		}
	}
}

// This is a bug demonstration test for endpoint '/cloud/user'
// Link to the issue: https://github.com/owncloud/ocis-ocs/issues/53

func TestGetUserSigningKey(t *testing.T) {
	user := User{
		Enabled:     "true",
		Username:    "rutherford",
		ID:          "rutherford",
		Email:       "rutherford@example.com",
		Displayname: "Ernest RutherFord",
		Password:    "password",
	}

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			err := createUser(user)
			if err != nil {
				t.Fatal(err)
			}

			formatpart := getFormatString(format)
			res, err := sendRequest(
				"GET",
				fmt.Sprintf("/%v/cloud/user/signing-key%v", ocsVersion, formatpart),
				"",
				fmt.Sprintf("%v:%v", user.Username, user.Password),
			)

			if err != nil {
				t.Fatal(err)
			}

			var response EmptyResponse

			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					log.Println(err)
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 400, res, ocsVersion)
			assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be a failure but was not")
			assertResponseMeta(t, Meta{
				Status:     "error",
				StatusCode: 400,
				Message:    "missing user in context",
			}, response.Ocs.Meta)
			assert.Empty(t, response.Ocs.Data)
			cleanUp(t)
		}
	}
}

func AddUserToGroup(userid, groupid string) error {
	res, err := sendRequest(
		"POST",
		fmt.Sprintf("/v2.php/cloud/users/%s/groups", userid),
		fmt.Sprintf("groupid=%v", groupid),
		"admin:admin",
	)
	if err != nil {
		return err
	}
	if res.Code != 200 {
		return fmt.Errorf("Failed while adding the user to group")
	}
	return nil
}

func TestListUsersGroupNewUsers(t *testing.T) {
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

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, user := range users {
				err := createUser(user)
				if err != nil {
					t.Fatal(err)
				}

				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%s/cloud/users/%s/groups%s", ocsVersion, user.ID, formatpart),
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response GetUsersGroupsResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 200, res, ocsVersion)
				assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
				assert.Empty(t, response.Ocs.Data.Groups)

				cleanUp(t)
			}
		}
	}
}

func TestListUsersGroupDefaultUsers(t *testing.T) {
	DefaultGroups := map[string][]string{
		"4c510ada-c86b-4815-8820-42cdf82c3d51": {
			"509a9dcd-bb37-4f4f-a01a-19dca27d9cfa",
			"6040aa17-9c64-4fef-9bd0-77234d71bad0",
			"dd58e5ec-842e-498b-8800-61f2ec6f911f",
			"262982c1-2362-4afa-bfdf-8cbfef64a06e",
		},
		"820ba2a1-3f54-4538-80a4-2d73007e30bf": {
			"34f38767-c937-4eb6-b847-1c175829a2a0",
		},
		"932b4540-8d16-481e-8ef4-588e4b6b151c": {
			"509a9dcd-bb37-4f4f-a01a-19dca27d9cfa",
			"a1726108-01f8-4c30-88df-2b1a9d1cba1a",
			"167cbee2-0518-455a-bfb2-031fe0621e5d",
			"262982c1-2362-4afa-bfdf-8cbfef64a06e",
		},
		"bc596f3c-c955-4328-80a0-60d018b4ad57": {
			"34f38767-c937-4eb6-b847-1c175829a2a0",
		},
		"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c": {
			"509a9dcd-bb37-4f4f-a01a-19dca27d9cfa",
			"7b87fd49-286e-4a5f-bafd-c535d5dd997a",
			"cedc21aa-4072-4614-8676-fa9165f598ff",
			"262982c1-2362-4afa-bfdf-8cbfef64a06e",
		},
		"058bff95-6708-4fe5-91e4-9ea3d377588b": {
			"509a9dcd-bb37-4f4f-a01a-19dca27d9cfa",
		},
	}

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, user := range DefaultUsers {
				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%s/cloud/users/%s/groups%s", ocsVersion, user, formatpart),
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response GetUsersGroupsResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 200, res, ocsVersion)
				assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")

				assert.Equal(t, DefaultGroups[user], response.Ocs.Data.Groups)
			}
		}
	}
	cleanUp(t)
}

func TestGetGroupForUserInvalidUserId(t *testing.T) {

	invalidUsers := []string{
		"1",
		"invalid",
		"3434234233",
		"1am41validUs3r",
		"_-@@--$$__",
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, user := range invalidUsers {
				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%s/cloud/users/%s/groups%s", ocsVersion, user, formatpart),
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 404, res, ocsVersion)
				assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
				assertResponseMeta(t, Meta{
					Status:     "error",
					StatusCode: 998,
					Message:    "The requested user could not be found",
				}, response.Ocs.Meta)

				assert.Empty(t, response.Ocs.Data)
			}
		}
	}
}

func TestAddUsersToGroupsNewUsers(t *testing.T) {
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

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, user := range users {
				err := createUser(user)
				if err != nil {
					t.Fatal(err)
				}

				// group id for Physics lover
				groupid := "262982c1-2362-4afa-bfdf-8cbfef64a06e"

				res, err := sendRequest(
					"POST",
					fmt.Sprintf("/%s/cloud/users/%s/groups%s", ocsVersion, user.ID, formatpart),
					"groupid="+groupid,
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 200, res, ocsVersion)
				assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
				assert.Empty(t, response.Ocs.Data)

				// Check the user is in the group
				res, err = sendRequest(
					"GET",
					fmt.Sprintf("/%s/cloud/users/%s/groups?format=json", ocsVersion, user.ID),
					"",
					"admin:admin",
				)
				if err != nil {
					t.Fatal(err)
				}
				var grpResponse GetUsersGroupsResponse
				if err := json.Unmarshal(res.Body.Bytes(), &grpResponse); err != nil {
					t.Fatal(err)
				}
				assert.Contains(t, grpResponse.Ocs.Data.Groups, groupid)

				cleanUp(t)
			}
		}
	}
}

// Issue: https://github.com/owncloud/ocis-ocs/issues/55 Incorrect message when adding user to non existing group
func TestAddUsersToGroupInvalidGroup(t *testing.T) {
	user := User{
		Enabled:     "true",
		Username:    "rutherford",
		ID:          "rutherford",
		Email:       "rutherford@example.com",
		Displayname: "Ernest RutherFord",
	}
	err := createUser(user)
	if err != nil {
		t.Fatal(err)
	}

	invalidGroups := []string{
		"1",
		"invalid",
		"3434234233",
		"1am41validUs3r",
		"_-@@--$$__",
		"c7fbe8c4-139b-4376-b307-cf0a8c2d0d9c",
	}

	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, groupid := range invalidGroups {
				res, err := sendRequest(
					"POST",
					fmt.Sprintf("/%s/cloud/users/rutherford/groups%s", ocsVersion, formatpart),
					"groupid="+groupid,
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 404, res, ocsVersion)
				assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be fail but was successful")
				assertResponseMeta(t, Meta{
					"error",
					998,
					"The requested user could not be found",
				}, response.Ocs.Meta)
				assert.Empty(t, response.Ocs.Data)
			}
		}
	}
	cleanUp(t)
}

// Issue: https://github.com/owncloud/ocis-ocs/issues/57 - cannot remove user from group
func TestRemoveUserFromGroup(t *testing.T) {
	user := User{
		Enabled:     "true",
		Username:    "rutherford",
		ID:          "rutherford",
		Email:       "rutherford@example.com",
		Displayname: "Ernest RutherFord",
	}

	groups := []string{
		"7b87fd49-286e-4a5f-bafd-c535d5dd997a", // radium-lovers
		"cedc21aa-4072-4614-8676-fa9165f598ff", // polonium-lovers
		"262982c1-2362-4afa-bfdf-8cbfef64a06e", // physics-lovers
	}

	var err error
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)

			err = createUser(user)
			if err != nil {
				t.Fatalf("Failed while creating new user: %v", err)
			}
			for _, group := range groups {
				err := AddUserToGroup(user.ID, group)
				if err != nil {
					t.Fatalf("Failed while creating new user: %v", err)
				}
			}

			// Remove user from one group
			res, err := sendRequest(
				"DELETE",
				fmt.Sprintf("/%s/cloud/users/%s/groups%s", ocsVersion, user.ID, formatpart),
				"groupid="+groups[0],
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var response EmptyResponse
			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 500, res, ocsVersion)
			assertResponseMeta(t, Meta{
				"error",
				996,
				"{\"id\":\".\",\"code\":500,\"detail\":\"could not clean up group id: invalid id .\",\"status\":\"Internal Server Error\"}",
			}, response.Ocs.Meta)
			assert.Empty(t, response.Ocs.Data)

			// Check the users are correctly added to group
			res, err = sendRequest(
				"GET",
				fmt.Sprintf("/%s/cloud/users/%s/groups?format=json", ocsVersion, user.ID),
				"",
				"admin:admin",
			)
			if err != nil {
				t.Fatal(err)
			}
			var grpResponse GetUsersGroupsResponse
			if err := json.Unmarshal(res.Body.Bytes(), &grpResponse); err != nil {
				t.Fatal(err)
			}

			// Change this line once the issue is fixed
			// assert.NotContains(t, grpResponse.Ocs.Data.Groups, groups[0])
			assert.Contains(t, grpResponse.Ocs.Data.Groups, groups[0])
			assert.Contains(t, grpResponse.Ocs.Data.Groups, groups[1])
			assert.Contains(t, grpResponse.Ocs.Data.Groups, groups[2])
			cleanUp(t)
		}
	}
}

func TestGetGroupsDefaultGroups(t *testing.T) {
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)

			res, err := sendRequest(
				"GET",
				fmt.Sprintf("/%s/cloud/groups%s", ocsVersion, formatpart),
				"",
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var response GetGroupsResponse
			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
					t.Fatal(err)
				}
			}

			assertStatusCode(t, 200, res, ocsVersion)
			assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but failed")
			assert.Equal(t, DefaultGroups, response.Ocs.Data.Groups)
		}
	}
}

func TestCreateGroup(t *testing.T) {
	testData := []struct {
		group Group
		err   *Meta
	}{
		// A simple group
		{
			Group{
				ID:          "grp1",
				GIDNumber:   32222,
				Displayname: "Group Name",
			},
			&Meta{
				Status:     "error",
				StatusCode: 999,
				Message:    "not implemented",
			},
		},
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			for _, data := range testData {
				formatpart := getFormatString(format)
				res, err := sendRequest(
					"POST",
					fmt.Sprintf("/%v/cloud/groups%v", ocsVersion, formatpart),
					data.group.getGroupRequestString(),
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse

				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				if data.err == nil {
					assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
				} else {
					assertResponseMeta(t, *data.err, response.Ocs.Meta)
				}

				// Check the group exists of not
				res, err = sendRequest(
					"GET",
					"/v2.php/cloud/groups?format=json",
					"",
					"admin:admin",
				)
				if err != nil {
					t.Fatal(err)
				}
				var groupResponse GetGroupsResponse
				if err := json.Unmarshal(res.Body.Bytes(), &groupResponse); err != nil {
					t.Fatal(err)
				}
				if data.err == nil {
					assert.Contains(t, groupResponse.Ocs.Data.Groups, data.group.ID)
				} else {
					assert.NotContains(t, groupResponse.Ocs.Data.Groups, data.group.ID)
				}
				cleanUp(t)
			}
		}
	}
}

// Add group not implemented
// Unskip this test after adding group is implemented.
func TestDeleteGroup(t *testing.T) { 
	t.Skip()
	testData := []Group{
		{
			ID:          "grp1",
			GIDNumber:   32222,
			Displayname: "Group Name",
		},
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, data := range testData {
				err := createGroup(data)

				res, err := sendRequest(
					"DELETE",
					fmt.Sprintf("/%v/cloud/groups/%v%v", ocsVersion, data.ID, formatpart),
					"groupid="+data.ID,
					"admin:admin",
				)
				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse
				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}
				assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")

				// Check the group does not exists
				res, err = sendRequest(
					"GET",
					"/v2.php/cloud/groups?format=json",
					"",
					"admin:admin",
				)
				if err != nil {
					t.Fatal(err)
				}
				var groupResponse GetGroupsResponse
				if err := json.Unmarshal(res.Body.Bytes(), &groupResponse); err != nil {
					t.Fatal(err)
				}

				assert.NotContains(t, groupResponse.Ocs.Data.Groups, data.ID)
				cleanUp(t)
			}
		}
	}
}

func TestDeleteGroupInvalidGroups(t *testing.T) {
	testData := []string{
		"1",
		"invalid",
		"3434234233",
		"1am41validUs3r",
		"_-@@--$$__",
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, data := range testData {
				res, err := sendRequest(
					"DELETE",
					fmt.Sprintf("/%v/cloud/groups/%v%v", ocsVersion, data, formatpart),
					"groupid="+data,
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse

				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 404, res, ocsVersion)
				assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to fail but was successful")
				assertResponseMeta(t, Meta{
					"error",
					998,
					"The requested group could not be found",
				}, response.Ocs.Meta)
				cleanUp(t)
			}
		}
	}
}

func TestGetGroupMembersDefaultGroups(t *testing.T) {
	defaultGroups := map[string][]string{
		"34f38767-c937-4eb6-b847-1c175829a2a0": {
			"820ba2a1-3f54-4538-80a4-2d73007e30bf", // konnectd
			"bc596f3c-c955-4328-80a0-60d018b4ad57", // reva
		},
		"509a9dcd-bb37-4f4f-a01a-19dca27d9cfa": {
			"4c510ada-c86b-4815-8820-42cdf82c3d51", // einstein
			"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c", // marie
			"932b4540-8d16-481e-8ef4-588e4b6b151c", // feynman
		},
		"6040aa17-9c64-4fef-9bd0-77234d71bad0": {
			"4c510ada-c86b-4815-8820-42cdf82c3d51", // einstein
		},
		"dd58e5ec-842e-498b-8800-61f2ec6f911f": {
			"4c510ada-c86b-4815-8820-42cdf82c3d51", // einstein
		},
		"cedc21aa-4072-4614-8676-fa9165f598ff": {
			"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c", // marie
		},
		"a1726108-01f8-4c30-88df-2b1a9d1cba1a": {
			"932b4540-8d16-481e-8ef4-588e4b6b151c", // feynman
		},
		"167cbee2-0518-455a-bfb2-031fe0621e5d": {
			"932b4540-8d16-481e-8ef4-588e4b6b151c", // feynman
		},
		"262982c1-2362-4afa-bfdf-8cbfef64a06e": {
			"4c510ada-c86b-4815-8820-42cdf82c3d51", // einstein
			"f7fbf8c8-139b-4376-b307-cf0a8c2d0d9c", // marie
			"932b4540-8d16-481e-8ef4-588e4b6b151c", // feynman
		},
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			for group, members := range defaultGroups {
				formatpart := getFormatString(format)
				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%v/cloud/groups/%v%v", ocsVersion, group, formatpart),
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response GetGroupMembersResponse

				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 200, res, ocsVersion)
				assert.True(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
				assert.Equal(t, members, response.Ocs.Data.Users)

				cleanUp(t)
			}
		}
	}
}


func TestListMembersInvalidGroups(t *testing.T) {
	testData := []string{
		"1",
		"invalid",
		"3434234233",
		"1am41validUs3r",
		"_-@@--$$__",
	}
	for _, ocsVersion := range ocsVersions {
		for _, format := range formats {
			formatpart := getFormatString(format)
			for _, group := range testData {
				res, err := sendRequest(
					"GET",
					fmt.Sprintf("/%v/cloud/groups/%v%v", ocsVersion, group, formatpart),
					"",
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response EmptyResponse

				if format == "json" {
					if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := xml.Unmarshal(res.Body.Bytes(), &response.Ocs); err != nil {
						t.Fatal(err)
					}
				}

				assertStatusCode(t, 404, res, ocsVersion)
				assert.False(t, response.Ocs.Meta.Success(ocsVersion), "The response was expected to fail but was successful")
				assertResponseMeta(t, Meta{
					"error",
					998,
					"The requested group could not be found",
				}, response.Ocs.Meta)
				cleanUp(t)
			}
		}
	}
}

type mockClient struct{}

func (c mockClient) Init(option ...client.Option) error {
	return nil
}

func (c mockClient) Options() client.Options {
	return client.Options{}
}

func (c mockClient) NewMessage(topic string, msg interface{}, opts ...client.MessageOption) client.Message {
	return nil
}

func (c mockClient) NewRequest(service, endpoint string, req interface{}, reqOpts ...client.RequestOption) client.Request {
	return nil
}

func (c mockClient) Call(ctx context.Context, req client.Request, rsp interface{}, opts ...client.CallOption) error {
	return nil
}

func (c mockClient) Stream(ctx context.Context, req client.Request, opts ...client.CallOption) (client.Stream, error) {
	return nil, nil
}

func (c mockClient) Publish(ctx context.Context, msg client.Message, opts ...client.PublishOption) error {
	return nil
}

func (c mockClient) String() string {
	return "ClientMock"
}
