package shares

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"path"
	"time"

	revagateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	revauser "github.com/cs3org/go-cs3apis/cs3/identity/user/v1beta1"
	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	revalink "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	revaprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	revatypes "github.com/cs3org/go-cs3apis/cs3/types/v1beta1"
	"github.com/cs3org/reva/pkg/appctx"
	"github.com/cs3org/reva/pkg/rgrpc/todo/pool"
	"github.com/go-chi/chi"
	"github.com/owncloud/ocis-ocs/pkg/config"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-pkg/v2/log"
	"github.com/pkg/errors"
)

// NewService returns a new Service instance
func NewService(logger log.Logger, conf *config.Sharing) (*Service, error) {
	gwc, err := pool.GetGatewayServiceClient(conf.RevaGatewayAddress)
	if err != nil {
		return nil, errors.Wrap(err, "could not get a reva client")
	}
	return &Service{
		logger:    logger,
		client:    gwc,
		publicURL: conf.PublicURL,
	}, nil
}

// Service implements sharing related functionality
type Service struct {
	logger    log.Logger
	client    revagateway.GatewayAPIClient
	publicURL string
}

// Routes returns a http handler handling the sharing routes
func (s *Service) Routes() http.Handler {
	r := chi.NewRouter()
	r.Route("/shares", func(r chi.Router) {
		r.Options("/", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		r.Get("/", s.listShares)
		r.Post("/", s.createShare)
		r.Route("/pending/{shareID}", func(r chi.Router) {
			r.Post("/", s.acceptShare)
			r.Delete("/", s.rejectShare)
		})
		r.Route("/remote_shares", func(r chi.Router) {
			r.Get("/", s.listFederatedShares)
			r.Get("/{shareID}", s.getFederatedShare)
		})
		r.Route("/{shareID}", func(r chi.Router) {
			r.Get("/", s.getShare)
			r.Put("/", s.updateShare)
			r.Delete("/", s.removeShare)
		})
	})

	return r
}

// TODO(jfd) merge userShare2ShareData with publicShare2ShareData
func userShare2ShareData(ctx context.Context, client revagateway.GatewayAPIClient, share *revacollaboration.Share) (*data.ShareData, error) {
	sd := &data.ShareData{
		Permissions: userSharePermissions2OCSPermissions(share.GetPermissions()),
		ShareType:   data.ShareTypeUser,
	}

	log := appctx.GetLogger(ctx)

	if share.Creator != nil {
		creator, err := client.GetUser(ctx, &revauser.GetUserRequest{
			UserId: share.Creator,
		})
		if err != nil {
			return nil, err
		}

		if creator.Status.Code == revarpc.Code_CODE_OK {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.UIDOwner = userIDToString(share.Creator)
			sd.DisplaynameOwner = creator.GetUser().DisplayName
		} else {
			log.Err(errors.Wrap(err, "could not look up creator")).
				Str("user_idp", share.Creator.GetIdp()).
				Str("user_opaque_id", share.Creator.GetOpaqueId()).
				Str("code", creator.Status.Code.String()).
				Msg(creator.Status.Message)
			return nil, err
		}
	}
	if share.Owner != nil {
		owner, err := client.GetUser(ctx, &revauser.GetUserRequest{
			UserId: share.Owner,
		})
		if err != nil {
			return nil, err
		}

		if owner.Status.Code == revarpc.Code_CODE_OK {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.UIDFileOwner = userIDToString(share.Owner)
			sd.DisplaynameFileOwner = owner.GetUser().DisplayName
		} else {
			log.Err(errors.Wrap(err, "could not look up owner")).
				Str("user_idp", share.Owner.GetIdp()).
				Str("user_opaque_id", share.Owner.GetOpaqueId()).
				Str("code", owner.Status.Code.String()).
				Msg(owner.Status.Message)
			return nil, err
		}
	}
	if share.Grantee.Id != nil {
		grantee, err := client.GetUser(ctx, &revauser.GetUserRequest{
			UserId: share.Grantee.GetId(),
		})
		if err != nil {
			return nil, err
		}

		if grantee.Status.Code == revarpc.Code_CODE_OK {
			// TODO the user from GetUser might not have an ID set, so we are using the one we have
			sd.ShareWith = userIDToString(share.Grantee.Id)
			sd.ShareWithDisplayname = grantee.GetUser().DisplayName
		} else {
			log.Err(errors.Wrap(err, "could not look up grantee")).
				Str("user_idp", share.Grantee.GetId().GetIdp()).
				Str("user_opaque_id", share.Grantee.GetId().GetOpaqueId()).
				Str("code", grantee.Status.Code.String()).
				Msg(grantee.Status.Message)
			return nil, err
		}
	}
	if share.Id != nil && share.Id.OpaqueId != "" {
		sd.ID = share.Id.OpaqueId
	}
	if share.Ctime != nil {
		sd.STime = share.Ctime.Seconds // TODO CS3 api birth time = btime
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
	return sd, nil
}

// UserSharePermissions2OCSPermissions transforms cs3api permissions into OCS Permissions data model
func userSharePermissions2OCSPermissions(sp *revacollaboration.SharePermissions) data.Permissions {
	if sp != nil {
		return permissions2OCSPermissions(sp.GetPermissions())
	}
	return data.PermissionInvalid
}

// TODO sort out mapping, this is just a first guess
// public link permissions to OCS permissions
func permissions2OCSPermissions(p *revaprovider.ResourcePermissions) data.Permissions {
	permissions := data.PermissionInvalid
	if p != nil {
		if p.ListContainer {
			permissions += data.PermissionRead
		}
		if p.InitiateFileUpload {
			permissions += data.PermissionWrite
		}
		if p.CreateContainer {
			permissions += data.PermissionCreate
		}
		if p.Delete {
			permissions += data.PermissionDelete
		}
		if p.AddGrant {
			permissions += data.PermissionShare
		}
	}
	return permissions
}

func userIDToString(userID *revauser.UserId) string {
	if userID == nil || userID.OpaqueId == "" {
		return ""
	}
	return userID.OpaqueId
}

func addFileInfo(ctx context.Context, client revagateway.GatewayAPIClient, s *data.ShareData, info *revaprovider.ResourceInfo) error {
	log := appctx.GetLogger(ctx)
	if info != nil {
		// TODO The owner is not set in the storage stat metadata ...
		parsedMt, _, err := mime.ParseMediaType(info.MimeType)
		if err != nil {
			// Should never happen. We log anyways so that we know if it happens.
			log.Warn().Err(err).Msg("failed to parse mimetype")
		}
		s.MimeType = parsedMt
		// TODO STime:     &types.Timestamp{Seconds: info.Mtime.Seconds, Nanos: info.Mtime.Nanos},
		s.StorageID = info.Id.StorageId
		// TODO Storage: int
		s.ItemSource = wrapResourceID(info.Id)
		s.FileSource = s.ItemSource
		s.FileTarget = path.Join("/", path.Base(info.Path))
		s.Path = path.Join("/", path.Base(info.Path)) // TODO hm this might have to be relative to the users home ... depends on the webdav_namespace config
		// TODO FileParent:
		// item type
		s.ItemType = data.ResourceType(info.GetType()).String()

		// file owner might not yet be set. Use file info
		if s.UIDFileOwner == "" {
			// TODO we don't know if info.Owner is always set.
			s.UIDFileOwner = userIDToString(info.Owner)
		}
		if s.DisplaynameFileOwner == "" && info.Owner != nil {
			owner, err := client.GetUser(ctx, &revauser.GetUserRequest{
				UserId: info.Owner,
			})
			if err != nil {
				return err
			}

			if owner.Status.Code == revarpc.Code_CODE_OK {
				// TODO the user from GetUser might not have an ID set, so we are using the one we have
				s.DisplaynameFileOwner = owner.GetUser().DisplayName
			} else {
				err := errors.New("could not look up share owner")
				log.Err(err).
					Str("user_idp", info.Owner.GetIdp()).
					Str("user_opaque_id", info.Owner.GetOpaqueId()).
					Str("code", owner.Status.Code.String()).
					Msg(owner.Status.Message)
				return err
			}
		}
		// share owner might not yet be set. Use file info
		if s.UIDOwner == "" {
			// TODO we don't know if info.Owner is always set.
			s.UIDOwner = userIDToString(info.Owner)
		}
		if s.DisplaynameOwner == "" && info.Owner != nil {
			owner, err := client.GetUser(ctx, &revauser.GetUserRequest{
				UserId: info.Owner,
			})

			if err != nil {
				return err
			}

			if owner.Status.Code == revarpc.Code_CODE_OK {
				// TODO the user from GetUser might not have an ID set, so we are using the one we have
				s.DisplaynameOwner = owner.User.DisplayName
			} else {
				err := errors.New("could not look up file owner")
				log.Err(err).
					Str("user_idp", info.Owner.GetIdp()).
					Str("user_opaque_id", info.Owner.GetOpaqueId()).
					Str("code", owner.Status.Code.String()).
					Msg(owner.Status.Message)
				return err
			}
		}
	}
	return nil
}

func wrapResourceID(r *revaprovider.ResourceId) string {
	return wrap(r.StorageId, r.OpaqueId)
}

// The fileID must be encoded
// - XML safe, because it is going to be used in the propfind result
// - url safe, because the id might be used in a url, eg. the /dav/meta nodes
// which is why we base64 encode it
func wrap(sid string, oid string) string {
	return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", sid, oid)))
}

func publicShare2ShareData(share *revalink.PublicShare, publicURL string) *data.ShareData {
	var expiration string
	if share.Expiration != nil {
		expiration = timestampToExpiration(share.Expiration)
	} else {
		expiration = ""
	}

	shareWith := ""
	if share.PasswordProtected {
		shareWith = "***redacted***"
	}

	return &data.ShareData{
		// share.permissions are mapped below
		// DisplaynameOwner:	 creator.DisplayName,
		// DisplaynameFileOwner: share.GetCreator().String(),
		ID:                   share.Id.OpaqueId,
		ShareType:            data.ShareTypePublicLink,
		ShareWith:            shareWith,
		ShareWithDisplayname: shareWith,
		STime:                share.Ctime.Seconds, // TODO CS3 api birth time = btime
		Token:                share.Token,
		Expiration:           expiration,
		MimeType:             share.Mtime.String(),
		Name:                 share.DisplayName,
		MailSend:             0,
		URL:                  publicURL + path.Join("/", "#/s/"+share.Token),
		Permissions:          publicSharePermissions2OCSPermissions(share.GetPermissions()),
		UIDOwner:             userIDToString(share.Creator),
		UIDFileOwner:         userIDToString(share.Owner),
	}
	// actually clients should be able to GET and cache the user info themselves ...
	// TODO check grantee type for user vs group
}

func publicSharePermissions2OCSPermissions(sp *revalink.PublicSharePermissions) data.Permissions {
	if sp != nil {
		return permissions2OCSPermissions(sp.GetPermissions())
	}
	return data.PermissionInvalid
}

// timestamp is assumed to be UTC ... just human readable ...
// FIXME and abiguous / error prone because there is no time zone ...
func timestampToExpiration(t *revatypes.Timestamp) string {
	return time.Unix(int64(t.Seconds), int64(t.Nanos)).UTC().Format("2006-01-02 15:05:05")
}

func map2CS3Permissions(role string, p data.Permissions) (*revaprovider.ResourcePermissions, error) {
	// TODO replace usage of this method with asCS3Permissions
	rp := &revaprovider.ResourcePermissions{
		ListContainer:        p.Contain(data.PermissionRead),
		ListGrants:           p.Contain(data.PermissionRead),
		ListFileVersions:     p.Contain(data.PermissionRead),
		ListRecycle:          p.Contain(data.PermissionRead),
		Stat:                 p.Contain(data.PermissionRead),
		GetPath:              p.Contain(data.PermissionRead),
		GetQuota:             p.Contain(data.PermissionRead),
		InitiateFileDownload: p.Contain(data.PermissionRead),

		// FIXME: uploader role with only write permission can use InitiateFileUpload, not anything else
		Move:               p.Contain(data.PermissionWrite),
		InitiateFileUpload: p.Contain(data.PermissionWrite),
		CreateContainer:    p.Contain(data.PermissionCreate),
		Delete:             p.Contain(data.PermissionDelete),
		RestoreFileVersion: p.Contain(data.PermissionWrite),
		RestoreRecycleItem: p.Contain(data.PermissionWrite),
		PurgeRecycle:       p.Contain(data.PermissionDelete),

		AddGrant:    p.Contain(data.PermissionShare),
		RemoveGrant: p.Contain(data.PermissionShare), // TODO when are you able to unshare / delete
		UpdateGrant: p.Contain(data.PermissionShare),
	}
	return rp, nil
}

func parseTimestamp(timestamp string) (*revatypes.Timestamp, error) {
	parsed, err := time.Parse("2006-01-02T15:04:05Z0700", timestamp)
	if err != nil {
		parsed, err = time.Parse("2006-01-02", timestamp)
	}
	if err != nil {
		return nil, errors.Wrap(err, "time parse failed")
	}
	final := parsed.UnixNano()

	return &revatypes.Timestamp{
		Seconds: uint64(final / 1_000_000_000),
		Nanos:   uint32(final / 1_000_000_000),
	}, nil
}
