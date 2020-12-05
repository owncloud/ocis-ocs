package shares

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path"
	"strconv"

	revagateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	revauser "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	invitepb "github.com/cs3org/go-cs3apis/cs3/ocm/invite/v1beta1"
	ocmprovider "github.com/cs3org/go-cs3apis/cs3/ocm/provider/v1beta1"
	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	revalink "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	ocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	revaprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revatypes "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
	"github.com/pkg/errors"
)

func (s *Service) createShare(w http.ResponseWriter, r *http.Request) {
	shareType, err := strconv.Atoi(r.FormValue("shareType"))
	if err != nil {
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, "shareType must be an integer"))
		return
	}

	switch shareType {
	case int(data.ShareTypeUser):
		s.createUserShare(w, r)
	case int(data.ShareTypePublicLink):
		s.createPublicLinkShare(w, r)
	case int(data.ShareTypeFederatedCloudShare):
		s.createFederatedCloudShare(w, r)
	default:
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, fmt.Sprintf("unknown share type %d", shareType)))
	}
}

func (s *Service) createUserShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// prefix the path with the owners home, because ocs share requests are relative to the home dir
	// TODO the path actually depends on the configured webdav_namespace
	hRes, err := s.client.GetHome(ctx, &revaprovider.GetHomeRequest{})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("get home: %s", err),
		))
		return
	}
	prefix := hRes.GetPath()
	sharepath := r.FormValue("path")
	// if user sharing is disabled
	// TODO this doesn't make any sense. If the gatewayAddr was not set then the client call before would have failed
	/*
		if h.gatewayAddr == "" {
			...
		}
	*/

	shareWith := r.FormValue("shareWith")
	if shareWith == "" {
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, "missing shareWith"))
	}

	userRes, err := s.client.GetUser(ctx, &revauser.GetUserRequest{
		UserId: &revauser.UserId{OpaqueId: shareWith},
	})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("get user: %s", err),
		))
		return
	}
	if userRes.Status.Code != revarpc.Code_CODE_OK {
		render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "user not found"))
		return
	}
	statRes, err := stat(ctx, s.client, path.Join(prefix, sharepath))
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaBadRequest.StatusCode,
			fmt.Sprintf("stat %s", err),
		))
		return
	}

	var permissions data.Permissions

	role := r.FormValue("role")
	// 2. if we don't have a role try to map the permissions
	if role == "" {
		pval := r.FormValue("permissions")
		if pval == "" {
			// default is all permissions / role coowner
			permissions = data.PermissionAll
			role = data.RoleCoowner
		} else {
			pint, err := strconv.Atoi(pval)
			if err != nil {
				render.Render(w, r, response.ErrRender(
					data.MetaBadRequest.StatusCode,
					"permissions must be an integer",
				))
				return
			}
			permissions, err = data.NewPermissions(pint)
			if err != nil {
				if err == data.ErrPermissionNotInRange {
					render.Render(w, r, response.ErrRender(http.StatusNotFound, err.Error()))
				} else {
					render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, err.Error()))
				}
				return
			}
			role = data.Permissions2Role(permissions)
		}
	}

	if statRes.Info != nil && statRes.Info.Type == revaprovider.ResourceType_RESOURCE_TYPE_FILE {
		// Single file shares should never have delete or create permissions
		permissions &^= data.PermissionCreate
		permissions &^= data.PermissionDelete
	}

	var resourcePermissions *revaprovider.ResourcePermissions
	resourcePermissions = asCS3Permissions(permissions, resourcePermissions)

	roleMap := map[string]string{"name": role}
	val, err := json.Marshal(roleMap)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("encode role: %s", err),
		))
		return
	}

	if statRes.Status.Code != revarpc.Code_CODE_OK {
		if statRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("stat: %s", err),
		))
		return
	}

	createShareReq := &revacollaboration.CreateShareRequest{
		Opaque: &revatypes.Opaque{
			Map: map[string]*revatypes.OpaqueEntry{
				"role": {
					Decoder: "json",
					Value:   val,
				},
			},
		},
		ResourceInfo: statRes.Info,
		Grant: &revacollaboration.ShareGrant{
			Grantee: &revaprovider.Grantee{
				Type: revaprovider.GranteeType_GRANTEE_TYPE_USER,
				Id:   userRes.User.GetId(),
			},
			Permissions: &revacollaboration.SharePermissions{
				Permissions: resourcePermissions,
			},
		},
	}

	createShareResponse, err := s.client.CreateShare(ctx, createShareReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("create share: %s", err),
		))
		return
	}
	if createShareResponse.Status.Code != revarpc.Code_CODE_OK {
		if createShareResponse.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("create share: %s", err),
		))
		return
	}
	sd, err := userShare2ShareData(ctx, s.client, createShareResponse.Share)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("map user share to share data: %s", err),
		))
		return
	}
	err = addFileInfo(ctx, s.client, sd, statRes.Info)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("add file info: %s", err),
		))
		return
	}

	render.Render(w, r, response.DataRender(s))
}

func (s *Service) createPublicLinkShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hRes, err := s.client.GetHome(ctx, &revaprovider.GetHomeRequest{})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("get home: %s", err),
		))
		return
	}

	prefix := hRes.GetPath()

	statReq := revaprovider.StatRequest{
		Ref: &revaprovider.Reference{
			Spec: &revaprovider.Reference_Path{
				Path: path.Join(prefix, r.FormValue("path")),
			},
		},
	}

	statRes, err := s.client.Stat(ctx, &statReq)
	if err != nil {
		s.logger.Debug().Err(err).Msg("stat failed")
	}

	if statRes.Status.Code != revarpc.Code_CODE_OK {
		if statRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(
				data.MetaNotFound.StatusCode,
				fmt.Sprintf("resource not found: %s", err),
			))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("stat: %s", err),
		))
		return
	}

	err = r.ParseForm()
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaBadRequest.StatusCode,
			fmt.Sprintf("parse form: %s", err),
		))
		return
	}

	newPermissions, err := permissionsFromRequest(r)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("ocPublicPermToCs3 %s", err),
		))
		return
	}

	if newPermissions == nil {
		// default perms: read-only
		// TODO: the default might change depending on allowed permissions and configs
		newPermissions, err = ocPublicPermToCs3(1)
		if err != nil {
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("ocPublicPermToCs3 with default permissions: %s", err),
			))
			return
		}
	}

	req := revalink.CreatePublicShareRequest{
		ResourceInfo: statRes.GetInfo(),
		Grant: &revalink.Grant{
			Permissions: &revalink.PublicSharePermissions{
				Permissions: newPermissions,
			},
			Password: r.FormValue("password"),
		},
	}

	expireTimeString, ok := r.Form["expireDate"]
	if ok {
		if expireTimeString[0] != "" {
			expireTime, err := parseTimestamp(expireTimeString[0])
			if err != nil {
				render.Render(w, r, response.ErrRender(
					data.MetaServerError.StatusCode,
					fmt.Sprintf("parseTimestamp: %s", err),
				))
				return
			}
			if expireTime != nil {
				req.Grant.Expiration = expireTime
			}
		}
	}

	// set displayname and password protected as arbitrary metadata
	req.ResourceInfo.ArbitraryMetadata = &revaprovider.ArbitraryMetadata{
		Metadata: map[string]string{
			"name": r.FormValue("name"),
			// "password": r.FormValue("password"),
		},
	}

	createRes, err := s.client.CreatePublicShare(ctx, &req)
	if err != nil || createRes.Status.Code != revarpc.Code_CODE_OK {
		s.logger.Debug().Err(err).Msgf("error creating a public share to resource id: %v", statRes.Info.GetId())
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("create public share: %s", err),
		))
		return
	}

	sd := publicShare2ShareData(createRes.Share, "")
	err = addFileInfo(ctx, s.client, sd, statRes.Info)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("addFileInfo: %s", err),
		))
		return
	}

	render.Render(w, r, response.DataRender(sd))
}

func (s *Service) createFederatedCloudShare(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// prefix the path with the owners home, because ocs share requests are relative to the home dir
	// TODO the path actually depends on the configured webdav_namespace
	hRes, err := s.client.GetHome(ctx, &revaprovider.GetHomeRequest{})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("get home: %s", err),
		))
		return
	}

	prefix := hRes.GetPath()

	shareWithUser, shareWithProvider := r.FormValue("shareWithUser"), r.FormValue("shareWithProvider")
	if shareWithUser == "" || shareWithProvider == "" {
		render.Render(w, r, response.ErrRender(
			data.MetaBadRequest.StatusCode,
			"missing shareWith parameters",
		))
		return
	}

	providerInfoResp, err := s.client.GetInfoByDomain(ctx, &ocmprovider.GetInfoByDomainRequest{
		Domain: shareWithProvider,
	})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("get info by domain: %s", err),
		))
		return
	}

	remoteUserRes, err := s.client.GetRemoteUser(ctx, &invitepb.GetRemoteUserRequest{
		RemoteUserId: &revauser.UserId{OpaqueId: shareWithUser, Idp: shareWithProvider},
	})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("get remote user: %s", err),
		))
		return
	}
	if remoteUserRes.Status.Code != revarpc.Code_CODE_OK {
		// TODO(corby) probably should check if the status code is not found but for now we copy the code from reva
		render.Render(w, r, response.ErrRender(
			data.MetaNotFound.StatusCode,
			"user not found",
		))
		return
	}

	var permissions data.Permissions
	var role string

	pval := r.FormValue("permissions")
	if pval == "" {
		// by default pnly allow read permissions / assign viewer role
		permissions = data.PermissionRead
		role = data.RoleViewer
	} else {
		pint, err := strconv.Atoi(pval)
		if err != nil {
			render.Render(w, r, response.ErrRender(
				data.MetaBadRequest.StatusCode,
				fmt.Sprintf("parse permissions: %s", err),
			))
			return
		}
		permissions, err = data.NewPermissions(pint)
		if err != nil {
			render.Render(w, r, response.ErrRender(
				data.MetaBadRequest.StatusCode,
				fmt.Sprintf("new permissions: %s", err),
			))
			return
		}
		role = data.Permissions2Role(permissions)
	}

	resourcePermissions, err := map2CS3Permissions(role, permissions)
	if err != nil {
		s.logger.Warn().Err(err).Msg("unknown role, mapping legacy permissions")
		resourcePermissions = asCS3Permissions(permissions, nil)
	}

	wrapped := map[string]string{"name": strconv.Itoa(int(permissions))}
	val, err := json.Marshal(wrapped)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("encode role: %s", err),
		))
		return
	}

	statReq := &revaprovider.StatRequest{
		Ref: &revaprovider.Reference{
			Spec: &revaprovider.Reference_Path{
				Path: path.Join(prefix, r.FormValue("path")),
			},
		},
	}

	statRes, err := s.client.Stat(ctx, statReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("stat: %s", err),
		))
		return
	}
	if statRes.Status.Code != revarpc.Code_CODE_OK {
		if statRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(
				data.MetaNotFound.StatusCode,
				"not found",
			))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("stat: %s", err),
		))
		return
	}

	createShareReq := &ocm.CreateOCMShareRequest{
		Opaque: &revatypes.Opaque{
			Map: map[string]*revatypes.OpaqueEntry{
				"permissions": {
					Decoder: "json",
					Value:   val,
				},
				"name": {
					Decoder: "plain",
					Value:   []byte(path.Base(statRes.Info.Path)),
				},
			},
		},
		ResourceId: statRes.Info.Id,
		Grant: &ocm.ShareGrant{
			Grantee: &revaprovider.Grantee{
				Type: revaprovider.GranteeType_GRANTEE_TYPE_USER,
				Id:   remoteUserRes.RemoteUser.GetId(),
			},
			Permissions: &ocm.SharePermissions{
				Permissions: resourcePermissions,
			},
		},
		RecipientMeshProvider: providerInfoResp.ProviderInfo,
	}

	createShareResponse, err := s.client.CreateOCMShare(ctx, createShareReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("create ocm share: %s", err),
		))
		return
	}
	if createShareResponse.Status.Code != revarpc.Code_CODE_OK {
		if createShareResponse.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(
				data.MetaNotFound.StatusCode,
				"not found",
			))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("create ocm share: %s", err),
		))
		return
	}

	render.Render(w, r, response.DataRender("OCM Share created"))
}

func stat(ctx context.Context, client revagateway.GatewayAPIClient, path string) (*revaprovider.StatResponse, error) {
	statReq := &revaprovider.StatRequest{
		Ref: &revaprovider.Reference{
			Spec: &revaprovider.Reference_Path{
				Path: path,
			},
		},
	}

	statRes, err := client.Stat(ctx, statReq)
	if err != nil {
		return nil, errors.Wrap(err, "error sending a grpc stat request")
	}
	return statRes, nil
}

// TODO sort out mapping, this is just a first guess
// TODO use roles to make this configurable
func asCS3Permissions(p data.Permissions, rp *revaprovider.ResourcePermissions) *revaprovider.ResourcePermissions {
	if rp == nil {
		rp = &revaprovider.ResourcePermissions{}
	}

	if p.Contain(data.PermissionRead) {
		rp.ListContainer = true
		rp.ListGrants = true
		rp.ListFileVersions = true
		rp.ListRecycle = true
		rp.Stat = true
		rp.GetPath = true
		rp.GetQuota = true
		rp.InitiateFileDownload = true
	}
	if p.Contain(data.PermissionWrite) {
		rp.InitiateFileUpload = true
		rp.RestoreFileVersion = true
		rp.RestoreRecycleItem = true
	}
	if p.Contain(data.PermissionCreate) {
		rp.CreateContainer = true
		// FIXME permissions mismatch: double check create vs write file
		rp.InitiateFileUpload = true
		if p.Contain(data.PermissionWrite) {
			rp.Move = true // TODO move only when create and write?
		}
	}
	if p.Contain(data.PermissionDelete) {
		rp.Delete = true
		rp.PurgeRecycle = true
	}
	if p.Contain(data.PermissionShare) {
		rp.AddGrant = true
		rp.RemoveGrant = true // TODO when are you able to unshare / delete
		rp.UpdateGrant = true
	}
	return rp
}

func permissionsFromRequest(r *http.Request) (*revaprovider.ResourcePermissions, error) {
	var err error
	// phoenix sends: {"permissions": 15}. See ocPublicPermToRole struct for mapping

	permKey := 1

	// note: "permissions" value has higher priority than "publicUpload"

	//handle legacy "publicUpload" arg that overrides permissions differently depending on the scenario
	// https://github.com/owncloud/core/blob/v10.4.0/apps/files_sharing/lib/Controller/Share20OcsController.php#L447
	publicUploadString, ok := r.Form["publicUpload"]
	if ok {
		publicUploadFlag, err := strconv.ParseBool(publicUploadString[0])
		if err != nil {
			return nil, errors.Wrap(err, "parsing publicUploadFlag failed")
		}
		if publicUploadFlag {
			// all perms except reshare
			permKey = 15
		}
	} else {
		permissionsString, ok := r.Form["permissions"]
		if !ok {
			return nil, nil
		}

		permKey, err = strconv.Atoi(permissionsString[0])
		if err != nil {
			return nil, errors.Wrap(err, "parsing permissionsString failed")
		}
	}

	p, err := ocPublicPermToCs3(permKey)
	if err != nil {
		return nil, errors.Wrap(err, "ocPublicPermToCs3 failed")
	}
	return p, err
}

func ocPublicPermToCs3(permKey int) (*revaprovider.ResourcePermissions, error) {
	role, ok := ocPublicPermToRole[permKey]
	if !ok {
		return nil, fmt.Errorf("role to permKey %d not found", permKey)
	}
	perm, err := data.NewPermissions(permKey)
	if err != nil {
		return nil, errors.Wrap(err, "creating permissions failed")
	}

	p, err := map2CS3Permissions(role, perm)
	if err != nil {
		return nil, errors.Wrap(err, "role to cs3permission failed")
	}

	return p, nil
}

// Maps oc10 public link permissions to roles
var ocPublicPermToRole = map[int]string{
	// Recipients can view and download contents.
	1: "viewer",
	// Recipients can view, download, edit, delete and upload contents
	15: "editor",
	// Recipients can upload but existing contents are not revealed
	4: "uploader",
	// Recipients can view, download and upload contents
	5: "contributor",
}
