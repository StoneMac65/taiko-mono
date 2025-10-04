package http

import (
	"net/http"
	"strconv"

	"github.com/cyberhorsey/webutils"
	"github.com/labstack/echo/v4"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer"
	"github.com/taikoxyz/taiko-mono/packages/eventindexer/pkg/repo"
)

// GetEpochGasTracking
//
//	returns epoch gas tracking data
//
//			@Summary		Get epoch gas tracking data
//			@ID			   	get-epoch-gas-tracking
//		    @Param			chainId		query		string		true	"chain ID (167000=mainnet, 167012=hoodi)"
//		    @Param			epochId		query		string		false	"specific epoch ID to get"
//		    @Param			startEpoch	query		string		false	"start epoch ID for range query"
//		    @Param			endEpoch	query		string		false	"end epoch ID for range query"
//		    @Param			page		query		string		false	"page number (default: 1)"
//		    @Param			size		query		string		false	"page size (default: 10, max: 100)"
//			@Accept			json
//			@Produce		json
//			@Success		200	{object} eventindexer.EpochGasResponse
//			@Router			/epochGas [get]
func (srv *Server) GetEpochGasTracking(c echo.Context) error {
	// Parse query parameters
	chainIDStr := c.QueryParam("chainId")
	if chainIDStr == "" {
		return webutils.LogAndRenderErrors(c, http.StatusBadRequest, 
			webutils.NewError("chainId is required"))
	}

	chainID, err := strconv.ParseUint(chainIDStr, 10, 64)
	if err != nil {
		return webutils.LogAndRenderErrors(c, http.StatusBadRequest, 
			webutils.NewError("invalid chainId"))
	}

	// Parse optional parameters
	var epochID *uint64
	if epochIDStr := c.QueryParam("epochId"); epochIDStr != "" {
		parsed, err := strconv.ParseUint(epochIDStr, 10, 64)
		if err != nil {
			return webutils.LogAndRenderErrors(c, http.StatusBadRequest, 
				webutils.NewError("invalid epochId"))
		}
		epochID = &parsed
	}

	var startEpoch *uint64
	if startEpochStr := c.QueryParam("startEpoch"); startEpochStr != "" {
		parsed, err := strconv.ParseUint(startEpochStr, 10, 64)
		if err != nil {
			return webutils.LogAndRenderErrors(c, http.StatusBadRequest, 
				webutils.NewError("invalid startEpoch"))
		}
		startEpoch = &parsed
	}

	var endEpoch *uint64
	if endEpochStr := c.QueryParam("endEpoch"); endEpochStr != "" {
		parsed, err := strconv.ParseUint(endEpochStr, 10, 64)
		if err != nil {
			return webutils.LogAndRenderErrors(c, http.StatusBadRequest, 
				webutils.NewError("invalid endEpoch"))
		}
		endEpoch = &parsed
	}

	// Parse pagination parameters
	page := 1
	if pageStr := c.QueryParam("page"); pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}

	size := 10
	if sizeStr := c.QueryParam("size"); sizeStr != "" {
		if parsed, err := strconv.Atoi(sizeStr); err == nil && parsed > 0 && parsed <= 100 {
			size = parsed
		}
	}

	// Create epoch gas tracking repository
	epochRepo, err := repo.NewEpochGasTrackingRepository(srv.db)
	if err != nil {
		return webutils.LogAndRenderErrors(c, http.StatusInternalServerError, err)
	}

	// Build query options
	opts := eventindexer.FindEpochGasTrackingOpts{
		ChainID:    chainID,
		EpochID:    epochID,
		StartEpoch: startEpoch,
		EndEpoch:   endEpoch,
		Limit:      size,
		Offset:     (page - 1) * size,
	}

	// Get epoch gas tracking data
	data, err := epochRepo.FindByEpochRange(c.Request().Context(), opts)
	if err != nil {
		return webutils.LogAndRenderErrors(c, http.StatusInternalServerError, err)
	}

	// Get total count for pagination
	totalCount, err := epochRepo.GetTotalCount(c.Request().Context(), chainID)
	if err != nil {
		return webutils.LogAndRenderErrors(c, http.StatusInternalServerError, err)
	}

	// Build response
	response := eventindexer.EpochGasResponse{
		Data:       data,
		TotalCount: totalCount,
		Page:       page,
		Size:       size,
	}

	return c.JSON(http.StatusOK, response)
}
