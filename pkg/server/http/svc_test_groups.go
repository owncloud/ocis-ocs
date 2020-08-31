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

	accountsCmd "github.com/owncloud/ocis-accounts/pkg/command"
	accountsCfg "github.com/owncloud/ocis-accounts/pkg/config"
	accountsProto "github.com/owncloud/ocis-accounts/pkg/proto/v0"
	accountsSvc "github.com/owncloud/ocis-accounts/pkg/service/v0"
)

var services = grpc.Service{}
var ocsVersion = []string{"v1.php", "v2.php"}
var format = []string{"json", "xml"}

func getFormatStringg(format string) string {
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

type meta struct {
	Status     string `json:"status" xml:"status"`
	StatusCode int    `json:"statuscode" xml:"statuscode"`
	Message    string `json:"message" xml:"message"`
}

func (m *meta) Success(ocsVersion string) bool {
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
	Meta meta `json:"meta" xml:"meta"`
	Data Group `json:"data" xml:"data"`
}

type GetGroupsResponse struct {
	Meta meta `json:"meta" xml:"meta"`
	Data struct {
		Groups []string `json:"groups" xml:"groups>element"`
	} `json:"data" xml:"data"`
}

type DeleteGroupRespone struct {
	Meta meta `json:"meta" xml:"meta"`
	Data struct {
	} `json:"data" xml:"data"`
}

func AssertRespMeta(t *testing.T, expected, actual meta) {
	assert.Equal(t, expected.Status, actual.Status, "The status of response doesn't match")
	assert.Equal(t, expected.StatusCode, actual.StatusCode, "The Status code of response doesn't match")
	assert.Equal(t, expected.Message, actual.Message, "The Message of response doesn't match")
}

func createGroup(g Group) error {
	_, err := sendReq(
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
		err  *meta
	}{
		// A simple group
		{
			Group{
				Enabled: "true",
				GroupID: "simpleGroup",
				UserID:  "testUser",
			},
			nil,
		},
	}
	for _, ocsVersion := range ocsVersion {
		for _, format := range format {
			for _, data := range testData {
				formatpart := getFormatStringg(format)
				res, err := sendReq(
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
					AssertRespMeta(t, *data.err, response.Meta)
				}

				res, err = sendReq(
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

	for _, ocsVersion := range ocsVersion {
		for _, format := range format {
			for _, group := range groups {
				err := createGroup(group)
				if err != nil {
					t.Fatal(err)
				}
			}

			formatpart := getFormatStringg(format)
			res, err := sendReq(
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
			res, err = sendReq(
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
	services = grpc.NewService(
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

	err = accountsProto.RegisterAccountsServiceHandler(services.Server(), hdlr)
	if err != nil {
		log.Fatal("could not register the Accounts handler")
	}
	err = accountsProto.RegisterGroupsServiceHandler(services.Server(), hdlr)
	if err != nil {
		log.Fatal("could not register the Groups handler")
	}

	err = services.Server().Start()
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
	client := services.Client()
	cl := accountsProto.NewGroupsService("com.owncloud.api.accounts", client)

	req := &accountsProto.DeleteGroupRequest{Id: id}
	res, err := cl.DeleteGroup(context.Background(),req)
	return res, err
}

func GetService() svc.Service {
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

func sendReq(method, endpoint, body, auth string) (*httptest.ResponseRecorder, error) {
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

	service := GetService()
	service.ServeHTTP(rr, req)

	return rr, nil
}
