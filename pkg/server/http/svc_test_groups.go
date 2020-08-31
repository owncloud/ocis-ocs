package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/owncloud/ocis-ocs/pkg/config"
	svc "github.com/owncloud/ocis-ocs/pkg/service/v0"
	ocisLog "github.com/owncloud/ocis-pkg/v2/log"
	"github.com/owncloud/ocis-pkg/v2/service/grpc"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/owncloud/ocis-ocs/pkg/config"
	_ "github.com/owncloud/ocis-ocs/pkg/service/v0"
	_ "github.com/owncloud/ocis-pkg/v2/log"
	_ "github.com/stretchr/testify/assert"

	accountsCmd "github.com/owncloud/ocis-accounts/pkg/command"
	accountsCfg "github.com/owncloud/ocis-accounts/pkg/config"
	accountsProto "github.com/owncloud/ocis-accounts/pkg/proto/v0"
	accountsSvc "github.com/owncloud/ocis-accounts/pkg/service/v0"
)

var Service = grpc.Service{}
var OcsVersions = []string{"v1.php", "v2.php"}
var Formats = []string{"json", "xml"}

func get_Format_String(format string) string {
	if format == "json" {
		return "?format=json"
	} else if format == "xml" {
		return ""
	} else {
		panic("Invalid format received")
	}
}

type Group struct {
	Enabled string `json:"enabled" xml:"enabled"`
	UserID  string `json:"userid" xml:"userid"`
	GroupID string `json:"groupid" xml:"groupid"`
}

func (g *Group) getGroupRequestString() string {
	res := fmt.Sprintf("userid=%v&groupid=%v", g.UserID, g.GroupID)

	if g.UserID != "" {
		res = res + "&uidnumber=" + fmt.Sprint(g.UserID)
	}

	if g.GroupID != "" {
		res = res + "&gidnumber=" + fmt.Sprint(g.GroupID)
	}

	return res
}

type _Meta_ struct {
	Status     string `json:"status" xml:"status"`
	StatusCode int    `json:"statuscode" xml:"statuscode"`
	Message    string `json:"message" xml:"message"`
}

func (m *_Meta_) Success(ocsVersion string) bool {
	if !(ocsVersion == "v1.php" || ocsVersion == "v2.php") {
		return false
	}
	if m.Status != "ok" {
		return false
	}
	if (ocsVersion == "v1.php" && m.StatusCode != 100) {
		return false
	} else if (ocsVersion == "v2.php" && m.StatusCode != 200) {
		return false
	} else {
		return true
	}
}

type SingleGroupResponse struct {
	Meta _Meta_ `json:"meta" xml:"meta"`
	Data Group `json:"data" xml:"data"`
}

type GetGroupsResponse struct {
	Meta _Meta_ `json:"meta" xml:"meta"`
	Data struct {
		Groups []string `json:"groups" xml:"groups>element"`
	} `json:"data" xml:"data"`
}

type DeleteGroupRespone struct {
	Meta _Meta_ `json:"meta" xml:"meta"`
	Data struct {
	} `json:"data" xml:"data"`
}

func assert_Response_Meta(t *testing.T, expected, actual _Meta_) {
	assert.Equal(t, expected.Status, actual.Status, "The status of response doesn't match")
	assert.Equal(t, expected.StatusCode, actual.StatusCode, "The Status code of response doesn't match")
	assert.Equal(t, expected.Message, actual.Message, "The Message of response doesn't match")
}

func createGroup(g Group) error {
	_, err := send_Request(
		"POST",
		"/v1.php/cloud/groups?format=json",
		g.getGroupRequestString(),
		"admin:admin",
	)
	if err != nil {
		return err
	}
	return nil
}

func TestCreateGroup(t *testing.T) {
	testData := []struct {
		group Group
		err  *_Meta_
	}{
		// A simple group
		{
			Group{
				Enabled:     "true",
			    GroupID: "simpleGroup",
			    UserID: "testUser",
			},
			nil,
		},
	}
	for _, ocsVersion := range OcsVersions {
		for _, format := range Formats {
			for _, data := range testData {
				formatpart := get_Format_String(format)
				res, err := send_Request(
					"POST",
					fmt.Sprintf("/%v/cloud/groups%v", ocsVersion, formatpart),
					data.group.getGroupRequestString(),
					"admin:admin",
				)

				if err != nil {
					t.Fatal(err)
				}

				var response SingleGroupResponse

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
					assert.True(t, response.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
				} else {
					assert_Response_Meta(t, *data.err, response.Meta)
				}

				res, err = send_Request(
					"GET",
					"/v1.php/cloud/groups?format=json",
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

				assert.True(t, groupResponse.Meta.Success("v1.php"), "The response was expected to be successful but was not")

				if data.err == nil {
					assert.Contains(t, groupResponse.Data.Groups, data.group.GroupID)
					assert.Contains(t, groupResponse.Data.Groups, data.group.UserID)
				} else {
					assert.NotContains(t, groupResponse.Data.Groups, data.group.GroupID)
				}
				clean(t)
			}
		}
	}
}

func TestDeleteGroup(t *testing.T) {
	time.Sleep(time.Second * 1)
	groups := []Group{
		{
			Enabled:     "true",
			GroupID: "simpleGroup",
			UserID: "testUser1",
		},
		{
			Enabled:     "true",
			GroupID: "group2",
			UserID: "testUser2",
		},
	}

	for _, ocsVersion := range OcsVersions {
		for _, format := range Formats {
			for _, group := range groups {
				err := createGroup(group)
				if err != nil {
					t.Fatal(err)
				}
			}

			formatpart := get_Format_String(format)
			res, err := send_Request(
				"DELETE",
				fmt.Sprintf("/%s/cloud/groups/simpleGroup%s", ocsVersion, formatpart),
				"",
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var response DeleteGroupRespone
			if format == "json" {
				if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := xml.Unmarshal(res.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
			}

			assert.True(t, response.Meta.Success(ocsVersion), "The response was expected to be successful but was not")
			assert.Empty(t, response.Data)

			// Check deleted group doesn't exist and the other group does
			res, err = send_Request(
				"GET",
				"/v1.php/cloud/groups?format=json",
				"",
				"admin:admin",
			)

			if err != nil {
				t.Fatal(err)
			}

			var groupsResponse GetGroupsResponse
			if err := json.Unmarshal(res.Body.Bytes(), &groupsResponse); err != nil {
				t.Fatal(err)
			}

			assert.True(t, groupsResponse.Meta.Success("v1.php"), "The response was expected to be successful but was not")
			assert.Contains(t, groupsResponse.Data.Groups, "group2")
			assert.NotContains(t, groupsResponse.Data.Groups, "simpleGroup")

			clean(t)
		}
	}
}

func init() {
	Service = grpc.NewService(
		grpc.Namespace("com.owncloud.api"),
		grpc.Name("accounts"),
		grpc.Address("localhost:9180"),
	)

	cfg := accountsCfg.New()
	cfg.Server.AccountsDataPath = "./accounts-store"
	var hdlr *accountsSvc.Service
	var err error

	if hdlr, err = accountsSvc.New(
		accountsSvc.Logger(accountsCmd.NewLogger(cfg)),
		accountsSvc.Config(cfg)); err != nil {
		log.Fatalf("Could not create new service")
	}

	err = accountsProto.RegisterAccountsServiceHandler(Service.Server(), hdlr)
	if err != nil {
		log.Fatal("could not register the Accounts handler")
	}
	err = accountsProto.RegisterGroupsServiceHandler(Service.Server(), hdlr)
	if err != nil {
		log.Fatal("could not register the Groups handler")
	}

	err = Service.Server().Start()
	if err != nil {
		log.Fatal(err)
	}
}

func clean(t *testing.T) {
	datastore := filepath.Join("./accounts-store", "groups")

	files, err := ioutil.ReadDir(datastore)
	if err != nil {
		log.Fatal(err)
	}

	for _, f := range files {
			deleteGroup(t, f.Name())
	}
}

func deleteGroup(t *testing.T, id string) (*empty.Empty, error) {
	client := Service.Client()
	cl := accountsProto.NewGroupsService("com.owncloud.api.accounts", client)

	req := &accountsProto.DeleteGroupRequest{Id: id}
	res, err := cl.DeleteGroup(context.Background(),req)
	return res, err
}

func get_Service() svc.Service {
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

func send_Request(method, endpoint, body, auth string) (*httptest.ResponseRecorder, error) {
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

	service := get_Service()
	service.ServeHTTP(rr, req)

	return rr, nil
}
