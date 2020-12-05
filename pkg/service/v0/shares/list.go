package shares

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"strconv"

	revagateway "github.com/cs3org/go-cs3apis/cs3/gateway/v1beta1"
	revarpc "github.com/cs3org/go-cs3apis/cs3/rpc/v1beta1"
	revacollaboration "github.com/cs3org/go-cs3apis/cs3/sharing/collaboration/v1beta1"
	revalink "github.com/cs3org/go-cs3apis/cs3/sharing/link/v1beta1"
	revaprovider "github.com/cs3org/go-cs3apis/cs3/storage/provider/v1beta1"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
	"github.com/pkg/errors"
)

const (
	stateAll      string = "all"
	stateAccepted string = "0"
	statePending  string = "1"
	stateRejected string = "2"

	ocsStateAccepted int = 0
	ocsStatePending  int = 1
	ocsStateRejected int = 2
)

// ListShares implements the endpoint to list shares
func (s *Service) listShares(w http.ResponseWriter, r *http.Request) {
	sharedWithMe, err := isSharedWithMe(r)
	if err != nil {
		render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
		return
	}
	if sharedWithMe {
		s.listSharesWithMe(w, r)
		return
	}
	s.listSharesWithOthers(w, r)
}

func (s *Service) listSharesWithMe(w http.ResponseWriter, r *http.Request) {
	switch r.FormValue("state") {
	default:
		fallthrough
	case stateAccepted:
		// TODO implement accepted filter
	case statePending:
		// TODO implement pending filter
	case stateRejected:
		// TODO implement rejected filter
	case stateAll:
		// no filter
	}

	lrsReq := revacollaboration.ListReceivedSharesRequest{}
	lrsRes, err := s.client.ListReceivedShares(r.Context(), &lrsReq)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("list received shares: %s", err),
		))
		return
	}

	if lrsRes.Status.Code != revarpc.Code_CODE_OK {
		if lrsRes.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "not found"))
			return
		}
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("list received shares: %s", err),
		))
		return
	}

	shares := make([]*data.ShareData, 0)
	// TODO(refs) filter out "invalid" shares
	for _, rs := range lrsRes.GetShares() {
		statRequest := revaprovider.StatRequest{
			Ref: &revaprovider.Reference{
				Spec: &revaprovider.Reference_Id{
					Id: rs.Share.ResourceId,
				},
			},
		}

		statResponse, err := s.client.Stat(r.Context(), &statRequest)
		if err != nil {
			render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
			return
		}

		shareData, err := userShare2ShareData(r.Context(), s.client, rs.Share)
		if err != nil {
			render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
			return
		}

		switch rs.GetState() {
		case revacollaboration.ShareState_SHARE_STATE_PENDING:
			shareData.State = ocsStatePending
		case revacollaboration.ShareState_SHARE_STATE_ACCEPTED:
			shareData.State = ocsStateAccepted
		case revacollaboration.ShareState_SHARE_STATE_REJECTED:
			shareData.State = ocsStateRejected
		default:
			shareData.State = -1
		}

		err = addFileInfo(r.Context(), s.client, shareData, statResponse.Info)
		if err != nil {
			render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
			return
		}

		shares = append(shares, shareData)
	}

	render.Render(w, r, response.DataRender(shares))
}

func (s *Service) listSharesWithOthers(w http.ResponseWriter, r *http.Request) {
	filters := []*revacollaboration.ListSharesRequest_Filter{}
	linkFilters := []*revalink.ListPublicSharesRequest_Filter{}

	p := r.URL.Query().Get("path")
	if p != "" {
		hRes, err := s.client.GetHome(r.Context(), &revaprovider.GetHomeRequest{})
		if err != nil {
			render.Render(w, r, response.ErrRender(
				data.MetaServerError.StatusCode,
				fmt.Sprintf("get home: %s", err),
			))
			return
		}

		filters, linkFilters, err = addFilters(r.Context(), s.client, hRes.GetPath(), r.FormValue("path"))
		if err != nil {
			render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
			return
		}
	}

	userShares, err := s.listUserShares(r.Context(), filters)
	if err != nil {
		render.Render(w, r, response.ErrRender(data.MetaBadRequest.StatusCode, err.Error()))
		return
	}

	publicShares, err := s.listPublicShares(r.Context(), linkFilters)
	if err != nil {
		render.Render(w, r, response.ErrRender(data.MetaServerError.StatusCode, err.Error()))
		return
	}

	shares := append(userShares, publicShares...)

	render.Render(w, r, response.DataRender(shares))
}

func (s *Service) listUserShares(ctx context.Context, filters []*revacollaboration.ListSharesRequest_Filter) ([]*data.ShareData, error) {
	lsUserSharesRequest := revacollaboration.ListSharesRequest{
		Filters: filters,
	}

	ocsDataPayload := make([]*data.ShareData, 0)

	lsUserSharesResponse, err := s.client.ListShares(ctx, &lsUserSharesRequest)
	if err != nil || lsUserSharesResponse.Status.Code != revarpc.Code_CODE_OK {
		return nil, errors.Wrap(err, "could not list shares")
	}

	for _, us := range lsUserSharesResponse.Shares {
		share, err := userShare2ShareData(ctx, s.client, us)
		if err != nil {
			return nil, errors.Wrap(err, "could not map user share to sharedata")
		}

		statReq := &revaprovider.StatRequest{
			Ref: &revaprovider.Reference{
				Spec: &revaprovider.Reference_Id{Id: us.ResourceId},
			},
		}

		statResponse, err := s.client.Stat(ctx, statReq)
		if err != nil || statResponse.Status.Code != revarpc.Code_CODE_OK {
			return nil, errors.Wrap(err, "could not stat share target")
		}

		err = addFileInfo(ctx, s.client, share, statResponse.Info)
		if err != nil {
			return nil, errors.Wrap(err, "could not add file info to share")
		}

		ocsDataPayload = append(ocsDataPayload, share)
	}

	return ocsDataPayload, nil
}

func (s *Service) listPublicShares(ctx context.Context, filters []*revalink.ListPublicSharesRequest_Filter) ([]*data.ShareData, error) {
	req := revalink.ListPublicSharesRequest{
		Filters: filters,
	}

	res, err := s.client.ListPublicShares(ctx, &req)
	if err != nil {
		return nil, errors.Wrap(err, "could not list public shares")
	}

	ocsDataPayload := make([]*data.ShareData, 0)
	for _, share := range res.GetShare() {
		statRequest := &revaprovider.StatRequest{
			Ref: &revaprovider.Reference{
				Spec: &revaprovider.Reference_Id{
					Id: share.ResourceId,
				},
			},
		}

		statResponse, err := s.client.Stat(ctx, statRequest)
		if err != nil || statResponse.Status.Code != revarpc.Code_CODE_OK {
			return nil, errors.Wrap(err, "could not stat share target")
		}

		sData := publicShare2ShareData(share, s.publicURL)

		sData.Name = share.DisplayName

		if addFileInfo(ctx, s.client, sData, statResponse.Info) != nil {
			return nil, errors.Wrap(err, "could not add file info")
		}

		ocsDataPayload = append(ocsDataPayload, sData)
	}
	return ocsDataPayload, nil
}

func isSharedWithMe(r *http.Request) (bool, error) {
	v := r.FormValue("shared_with_me")
	if v == "" {
		return false, nil
	}
	listSharedWithMe, err := strconv.ParseBool(v)
	if err != nil {
		return false, errors.Wrap(err, "error mapping share data")
	}
	return listSharedWithMe, nil
}

func addFilters(ctx context.Context, client revagateway.GatewayAPIClient, prefix string, targetPath string) ([]*revacollaboration.ListSharesRequest_Filter, []*revalink.ListPublicSharesRequest_Filter, error) {
	collaborationFilters := []*revacollaboration.ListSharesRequest_Filter{}
	linkFilters := []*revalink.ListPublicSharesRequest_Filter{}

	target := path.Join(prefix, targetPath)

	statReq := &revaprovider.StatRequest{
		Ref: &revaprovider.Reference{
			Spec: &revaprovider.Reference_Path{
				Path: target,
			},
		},
	}

	res, err := client.Stat(ctx, statReq)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error sending a grpc stat request")
	}

	if res.Status.Code != revarpc.Code_CODE_OK {
		if res.Status.Code == revarpc.Code_CODE_NOT_FOUND {
			return nil, nil, errors.Wrap(err, "not found")
		}
		return nil, nil, errors.Wrap(err, "grpc stat request failed")
	}

	info := res.Info

	collaborationFilters = append(collaborationFilters, &revacollaboration.ListSharesRequest_Filter{
		Type: revacollaboration.ListSharesRequest_Filter_TYPE_RESOURCE_ID,
		Term: &revacollaboration.ListSharesRequest_Filter_ResourceId{
			ResourceId: info.Id,
		},
	})

	linkFilters = append(linkFilters, &revalink.ListPublicSharesRequest_Filter{
		Type: revalink.ListPublicSharesRequest_Filter_TYPE_RESOURCE_ID,
		Term: &revalink.ListPublicSharesRequest_Filter_ResourceId{
			ResourceId: info.Id,
		},
	})

	return collaborationFilters, linkFilters, nil
}
