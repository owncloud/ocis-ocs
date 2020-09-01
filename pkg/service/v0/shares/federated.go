package shares

import (
	"fmt"
	"net/http"

	revaocm "github.com/cs3org/go-cs3apis/cs3/sharing/ocm/v1beta1"
	"github.com/go-chi/chi"
	"github.com/go-chi/render"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/data"
	"github.com/owncloud/ocis-ocs/pkg/service/v0/response"
)

func (s *Service) listFederatedShares(w http.ResponseWriter, r *http.Request) {
	// TODO Implement pagination
	// TODO Implement response with HAL schemating
	ctx := r.Context()

	listOCMSharesResponse, err := s.client.ListOCMShares(ctx, &revaocm.ListOCMSharesRequest{})
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error sending a grpc list ocm share request: %s", err),
		))
		return
	}

	shares := listOCMSharesResponse.GetShares()
	if shares == nil {
		shares = make([]*revaocm.Share, 0)
	}
	render.Render(w, r, response.DataRender(shares))
}

func (s *Service) getFederatedShare(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement response with HAL schemating
	ctx := r.Context()

	shareID := chi.URLParam(r, "shareID")

	listOCMSharesRequest := &revaocm.GetOCMShareRequest{
		Ref: &revaocm.ShareReference{
			Spec: &revaocm.ShareReference_Id{
				Id: &revaocm.ShareId{
					OpaqueId: shareID,
				},
			},
		},
	}

	ocmShareResponse, err := s.client.GetOCMShare(ctx, listOCMSharesRequest)
	if err != nil {
		render.Render(w, r, response.ErrRender(
			data.MetaServerError.StatusCode,
			fmt.Sprintf("error sending a grpc get ocm share request: %s", err),
		))
		return
	}

	share := ocmShareResponse.GetShare()
	if share == nil {
		render.Render(w, r, response.ErrRender(data.MetaNotFound.StatusCode, "share not found"))
		return
	}
	render.Render(w, r, response.DataRender(share))
}
