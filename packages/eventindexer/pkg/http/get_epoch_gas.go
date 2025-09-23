package http

import (
	"fmt"
	"net/http"

	"github.com/cyberhorsey/webutils"
	"github.com/labstack/echo/v4"
	"github.com/patrickmn/go-cache"
)

// GetEpochRevenue
//
//	returns epoch L2 revenue data in ETH
//
//			@Summary		Get epoch L2 revenue data
//			@ID			   	get-epoch-revenue
//		    @Param			proposer	query		string		false	"proposer address to filter by"
//		    @Param			start_date	query		string		false	"start date (YYYY-MM-DD)"
//		    @Param			end_date	query		string		false	"end date (YYYY-MM-DD)"
//		    @Param			chain_id	query		string		false	"chain ID (167000=mainnet, 167009=hekla, 167008=tolba)"
//			@Accept			json
//			@Produce		json
//			@Success		200	{object} eventindexer.ChartResponse
//			@Router			/epoch-revenue [get]
func (srv *Server) GetEpochRevenue(c echo.Context) error {
	proposerAddress := c.QueryParam("proposer")
	startDate := c.QueryParam("start_date")
	endDate := c.QueryParam("end_date")
	chainID := c.QueryParam("chain_id")

	// Determine task name based on chain ID
	var taskName string
	if chainID != "" {
		taskName = fmt.Sprintf("epoch_l2_revenue_%s", chainID)
	} else {
		// Default to mainnet if no chain ID specified
		taskName = "epoch_l2_revenue_167000"
	}

	// Build cache key including chain ID
	cacheKey := fmt.Sprintf("epoch_revenue_%s_%s_%s_%s",
		chainID, proposerAddress, startDate, endDate)

	// Check cache
	if cached, found := srv.cache.Get(cacheKey); found {
		return c.JSON(http.StatusOK, cached)
	}

	// Get chart data from repository using the network-specific task
	chart, err := srv.chartRepo.Find(
		c.Request().Context(),
		taskName, // Network-specific task name
		startDate,
		endDate,
		proposerAddress, // Use as fee_token_address filter
		"",              // No tier filter
	)
	if err != nil {
		return webutils.LogAndRenderErrors(c, http.StatusUnprocessableEntity, err)
	}

	// Cache the response
	srv.cache.Set(cacheKey, chart, cache.DefaultExpiration)

	return c.JSON(http.StatusOK, chart)
}
