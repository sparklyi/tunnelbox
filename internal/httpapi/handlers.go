package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sparklyi/tunnelbox/internal/operation"
	"github.com/sparklyi/tunnelbox/internal/provision"
	"github.com/sparklyi/tunnelbox/internal/service"
)

type createServiceRequest struct {
	Name       string            `json:"name"`
	Hostname   string            `json:"hostname"`
	OriginURL  string            `json:"origin_url"`
	AllowType  service.AllowType `json:"allow_type"`
	AllowValue string            `json:"allow_value"`
}

type updateServiceRequest struct {
	Name       *string            `json:"name"`
	Hostname   *string            `json:"hostname"`
	OriginURL  *string            `json:"origin_url"`
	AllowType  *service.AllowType `json:"allow_type"`
	AllowValue *string            `json:"allow_value"`
}

type serviceResponse struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Hostname            string            `json:"hostname"`
	OriginURL           string            `json:"origin_url"`
	AllowType           service.AllowType `json:"allow_type"`
	AllowValue          string            `json:"allow_value"`
	State               service.State     `json:"state"`
	TunnelID            string            `json:"tunnel_id,omitempty"`
	DNSRecordID         string            `json:"dns_record_id,omitempty"`
	AccessApplicationID string            `json:"access_application_id,omitempty"`
	AccessPolicyID      string            `json:"access_policy_id,omitempty"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
}

type operationResponse struct {
	ID           string           `json:"operation_id"`
	ServiceID    string           `json:"service_id"`
	Kind         string           `json:"kind"`
	Status       operation.Status `json:"status"`
	CurrentStep  string           `json:"current_step,omitempty"`
	Attempts     int              `json:"attempts"`
	ErrorCode    string           `json:"error_code,omitempty"`
	ErrorMessage string           `json:"error_message,omitempty"`
	CreatedAt    string           `json:"created_at"`
	UpdatedAt    string           `json:"updated_at"`
	StartedAt    *string          `json:"started_at,omitempty"`
	FinishedAt   *string          `json:"finished_at,omitempty"`
}

type cloudflareConfigureRequest struct {
	AccountID string `json:"account_id"`
	ZoneID    string `json:"zone_id"`
	Token     string `json:"token"`
}

type cloudflareStatusResponse struct {
	Configured bool   `json:"configured"`
	AccountID  string `json:"account_id,omitempty"`
	ZoneID     string `json:"zone_id,omitempty"`
	TokenID    string `json:"token_id,omitempty"`
	TokenState string `json:"token_state,omitempty"`
	LastError  string `json:"last_error,omitempty"`
}

type zoneResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type connectorResponse struct {
	ServiceID string `json:"service_id"`
	Running   bool   `json:"running"`
	Healthy   bool   `json:"healthy"`
	Message   string `json:"message,omitempty"`
}

func configureCloudflareHandler(integration CloudflareIntegration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if integration == nil {
			writeError(c, http.StatusNotImplemented, "integration_unavailable", "Cloudflare integration is not configured")
			return
		}
		var request cloudflareConfigureRequest
		if !decodeJSON(c, &request) {
			return
		}
		status, err := integration.Configure(c.Request.Context(), provision.CloudflareConfigureInput{
			AccountID: request.AccountID, ZoneID: request.ZoneID, Token: request.Token,
		})
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusOK, makeCloudflareStatusResponse(status))
	}
}

func cloudflareStatusHandler(integration CloudflareIntegration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if integration == nil {
			writeError(c, http.StatusNotImplemented, "integration_unavailable", "Cloudflare integration is not configured")
			return
		}
		status, err := integration.Status(c.Request.Context())
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusOK, makeCloudflareStatusResponse(status))
	}
}

func listZonesHandler(integration CloudflareIntegration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if integration == nil {
			writeError(c, http.StatusNotImplemented, "integration_unavailable", "Cloudflare integration is not configured")
			return
		}
		zones, err := integration.Zones(c.Request.Context())
		if err != nil {
			writeDomainError(c, err)
			return
		}
		response := make([]zoneResponse, 0, len(zones))
		for _, zone := range zones {
			response = append(response, zoneResponse{ID: zone.ID, Name: zone.Name})
		}
		c.JSON(http.StatusOK, gin.H{"zones": response})
	}
}

func listConnectorsHandler(lister ConnectorLister) gin.HandlerFunc {
	return func(c *gin.Context) {
		if lister == nil {
			c.JSON(http.StatusOK, gin.H{"connectors": []connectorResponse{}})
			return
		}
		items, err := lister.List(c.Request.Context())
		if err != nil {
			writeDomainError(c, err)
			return
		}
		response := make([]connectorResponse, 0, len(items))
		for _, item := range items {
			response = append(response, connectorResponse{ServiceID: item.ServiceID, Running: item.Running, Healthy: item.Healthy, Message: item.Message})
		}
		c.JSON(http.StatusOK, gin.H{"connectors": response})
	}
}

func listServicesHandler(actions ServiceActions) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := actions.List(c.Request.Context())
		if err != nil {
			writeDomainError(c, err)
			return
		}
		response := make([]serviceResponse, 0, len(items))
		for _, item := range items {
			response = append(response, makeServiceResponse(item))
		}
		c.JSON(http.StatusOK, gin.H{"services": response})
	}
}

func createServiceHandler(actions ServiceActions) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request createServiceRequest
		if !decodeJSON(c, &request) {
			return
		}
		item, err := actions.Create(c.Request.Context(), service.CreateInput{
			Name: request.Name, Hostname: request.Hostname, OriginURL: request.OriginURL,
			AllowType: request.AllowType, AllowValue: request.AllowValue,
		})
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusCreated, makeServiceResponse(item))
	}
}

func getServiceHandler(actions ServiceActions) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, err := actions.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusOK, makeServiceResponse(item))
	}
}

func updateServiceHandler(actions ServiceActions) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request updateServiceRequest
		if !decodeJSON(c, &request) {
			return
		}
		item, err := actions.Update(c.Request.Context(), c.Param("id"), service.UpdateInput{
			Name: request.Name, Hostname: request.Hostname, OriginURL: request.OriginURL,
			AllowType: request.AllowType, AllowValue: request.AllowValue,
		})
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusOK, makeServiceResponse(item))
	}
}

func deleteServiceHandler(actions ServiceActions) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := actions.Delete(c.Request.Context(), c.Param("id")); err != nil {
			writeDomainError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func deployServiceHandler(deployer ServiceDeployer) gin.HandlerFunc {
	return func(c *gin.Context) {
		if deployer == nil {
			writeError(c, http.StatusNotImplemented, "deployment_unavailable", "deployment is not configured")
			return
		}
		op, err := deployer.Deploy(contextForOperation(c), c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, makeOperationResponse(op))
	}
}

func getOperationHandler(reader OperationReader) gin.HandlerFunc {
	return func(c *gin.Context) {
		op, err := reader.Get(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeDomainError(c, err)
			return
		}
		c.JSON(http.StatusOK, makeOperationResponse(op))
	}
}

func decodeJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_json", "request body is invalid")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeError(c, http.StatusBadRequest, "invalid_json", "request body must contain one JSON object")
		return false
	}
	return true
}

func makeServiceResponse(item service.Service) serviceResponse {
	return serviceResponse{
		ID: item.ID, Name: item.Name, Hostname: item.Hostname, OriginURL: item.OriginURL,
		AllowType: item.AllowType, AllowValue: item.AllowValue, State: item.State,
		TunnelID: item.TunnelID, DNSRecordID: item.DNSRecordID,
		AccessApplicationID: item.AccessApplicationID, AccessPolicyID: item.AccessPolicyID,
		CreatedAt: item.CreatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
		UpdatedAt: item.UpdatedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00"),
	}
}

func makeOperationResponse(item operation.Operation) operationResponse {
	response := operationResponse{
		ID: item.ID, ServiceID: item.ServiceID, Kind: item.Kind, Status: item.Status,
		CurrentStep: item.CurrentStep, Attempts: item.Attempts, ErrorCode: item.ErrorCode,
		ErrorMessage: item.ErrorMessage, CreatedAt: item.CreatedAt.UTC().Format(timeFormat),
		UpdatedAt: item.UpdatedAt.UTC().Format(timeFormat),
	}
	if item.StartedAt != nil {
		value := item.StartedAt.UTC().Format(timeFormat)
		response.StartedAt = &value
	}
	if item.FinishedAt != nil {
		value := item.FinishedAt.UTC().Format(timeFormat)
		response.FinishedAt = &value
	}
	return response
}

const timeFormat = "2006-01-02T15:04:05.999999999Z07:00"

func writeDomainError(c *gin.Context, err error) {
	var validation *service.ValidationError
	var coded provision.CodedError
	switch {
	case errors.As(err, &validation):
		writeError(c, http.StatusBadRequest, "invalid_request", validation.Error())
	case errors.As(err, &coded):
		code := safeErrorCode(coded.FailureCode())
		writeError(c, statusForCode(code), code, messageForCode(code))
	case errors.Is(err, service.ErrNotFound), errors.Is(err, operation.ErrNotFound):
		writeError(c, http.StatusNotFound, "not_found", "resource was not found")
	case errors.Is(err, service.ErrConflict), errors.Is(err, operation.ErrConflict):
		writeError(c, http.StatusConflict, "conflict", "resource is busy or already exists")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(c, http.StatusGatewayTimeout, "timeout", "operation timed out")
	default:
		writeError(c, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

func safeErrorCode(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || len(code) > 64 {
		return "internal_error"
	}
	for _, r := range code {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return "internal_error"
		}
	}
	return code
}

func statusForCode(code string) int {
	switch code {
	case "cloudflare_configuration_invalid":
		return http.StatusBadRequest
	case "cloudflare_not_configured", "connector_not_running":
		return http.StatusConflict
	case "origin_unreachable", "cloudflare_unavailable", "cloudflare_timeout", "connector_start_failed":
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

func messageForCode(code string) string {
	switch code {
	case "cloudflare_configuration_invalid":
		return "Cloudflare configuration is invalid"
	case "cloudflare_not_configured":
		return "Cloudflare integration is not configured"
	case "cloudflare_zone_not_available":
		return "the selected Cloudflare zone is not available"
	case "cloudflare_token_inactive":
		return "the Cloudflare API token is not active"
	case "cloudflare_token_path_unconfigured":
		return "Cloudflare token storage is not configured"
	case "connector_not_running":
		return "connector is not running"
	case "origin_unreachable":
		return "origin cannot be reached from connector"
	default:
		return "request could not be completed"
	}
}

func makeCloudflareStatusResponse(status provision.CloudflareIntegrationStatus) cloudflareStatusResponse {
	return cloudflareStatusResponse{Configured: status.Configured, AccountID: status.AccountID, ZoneID: status.ZoneID,
		TokenID: status.TokenID, TokenState: status.TokenState, LastError: status.LastError}
}

func writeError(c *gin.Context, status int, code, message string) {
	if c.Writer.Written() {
		return
	}
	requestID, _ := c.Get(requestIDKey)
	c.AbortWithStatusJSON(status, gin.H{
		"code": code, "message": strings.TrimSpace(message), "request_id": requestID,
	})
}
