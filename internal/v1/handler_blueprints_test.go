package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/osbuild/image-builder/internal/clients/composer"
	"github.com/osbuild/image-builder/internal/common"
	"github.com/osbuild/image-builder/internal/db"
	"github.com/stretchr/testify/require"

	"github.com/osbuild/image-builder/internal/tutils"
)

func TestHandlers_CreateBlueprint(t *testing.T) {
	var jsonResp HTTPErrorList
	ctx := context.Background()
	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	body := map[string]interface{}{
		"name":           "Blueprint",
		"description":    "desc",
		"customizations": map[string]interface{}{"packages": []string{"nginx"}},
		"distribution":   "centos-9",
		"image_requests": []map[string]interface{}{
			{
				"architecture":   "x86_64",
				"image_type":     "aws",
				"upload_request": map[string]interface{}{"type": "aws", "options": map[string]interface{}{"share_with_accounts": []string{"test-account"}}},
			},
		},
	}
	statusCode, _ := tutils.PostResponseBody(t, "http://localhost:8086/api/image-builder/v1/blueprints", body)
	require.Equal(t, http.StatusCreated, statusCode)

	// Test unique name constraint
	statusCode, resp := tutils.PostResponseBody(t, "http://localhost:8086/api/image-builder/v1/blueprints", body)
	require.Equal(t, http.StatusUnprocessableEntity, statusCode)
	err = json.Unmarshal([]byte(resp), &jsonResp)
	require.NoError(t, err)
	require.Equal(t, "Name not unique", jsonResp.Errors[0].Title)

	// Test non empty name constraint
	body["name"] = ""
	statusCode, resp = tutils.PostResponseBody(t, "http://localhost:8086/api/image-builder/v1/blueprints", body)
	require.Equal(t, http.StatusUnprocessableEntity, statusCode)
	err = json.Unmarshal([]byte(resp), &jsonResp)
	require.NoError(t, err)
	require.Equal(t, "Invalid blueprint name", jsonResp.Errors[0].Title)
}

func TestHandlers_UpdateBlueprint(t *testing.T) {
	var jsonResp HTTPErrorList
	ctx := context.Background()
	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	body := map[string]interface{}{
		"name":           "Blueprint",
		"description":    "desc",
		"customizations": map[string]interface{}{"packages": []string{"nginx"}},
		"distribution":   "centos-9",
		"image_requests": []map[string]interface{}{
			{
				"architecture":   "x86_64",
				"image_type":     "aws",
				"upload_request": map[string]interface{}{"type": "aws", "options": map[string]interface{}{"share_with_accounts": []string{"test-account"}}},
			},
		},
	}
	statusCode, resp := tutils.PostResponseBody(t, "http://localhost:8086/api/image-builder/v1/blueprints", body)
	require.Equal(t, http.StatusCreated, statusCode)
	var result ComposeResponse
	err = json.Unmarshal([]byte(resp), &result)
	require.NoError(t, err)

	// Test non empty name constraint
	body["name"] = ""
	statusCode, resp = tutils.PutResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s", result.Id), body)
	t.Log(resp)
	require.Equal(t, http.StatusUnprocessableEntity, statusCode)
	err = json.Unmarshal([]byte(resp), &jsonResp)
	require.NoError(t, err)
	require.Equal(t, "Invalid blueprint name", jsonResp.Errors[0].Title)

	body["name"] = "Changing to correct body"
	respStatusCodeNotFound, _ := tutils.PutResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/experimental/blueprints/%s", uuid.New()), body)
	require.Equal(t, http.StatusNotFound, respStatusCodeNotFound)
}

func TestHandlers_ComposeBlueprint(t *testing.T) {
	ctx := context.Background()

	ids := []uuid.UUID{}
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newId := uuid.New()
		if "Bearer" == r.Header.Get("Authorization") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		require.Equal(t, "Bearer accesstoken", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		result := composer.ComposeId{
			Id: newId,
		}
		ids = append(ids, newId)
		encodeErr := json.NewEncoder(w).Encode(result)
		require.NoError(t, encodeErr)
	}))
	defer apiSrv.Close()

	dbase, err := dbc.NewDB()
	require.NoError(t, err)
	srv, tokenSrv := startServer(t, &testServerClientsConf{ComposerURL: apiSrv.URL}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		shutdownErr := srv.Shutdown(ctx)
		require.NoError(t, shutdownErr)
	}()
	defer tokenSrv.Close()

	id := uuid.New()
	versionId := uuid.New()

	uploadOptions := UploadRequest_Options{}
	err = uploadOptions.FromAWSUploadRequestOptions(AWSUploadRequestOptions{
		ShareWithAccounts: common.ToPtr([]string{"test-account"}),
	})
	require.NoError(t, err)
	name := "Blueprint Human Name"
	description := "desc"
	blueprint := BlueprintBody{
		Customizations: Customizations{
			Packages: common.ToPtr([]string{"nginx"}),
		},
		Distribution: "centos-9",
		ImageRequests: []ImageRequest{
			{
				Architecture: ImageRequestArchitectureX8664,
				ImageType:    ImageTypesAws,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAws,
					Options: uploadOptions,
				},
			},
			{
				Architecture: ImageRequestArchitectureAarch64,
				ImageType:    ImageTypesAws,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAws,
					Options: uploadOptions,
				},
			},
			{
				Architecture: ImageRequestArchitectureAarch64,
				ImageType:    ImageTypesGuestImage,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAwsS3,
					Options: uploadOptions,
				},
			},
		},
	}
	var message []byte
	message, err = json.Marshal(blueprint)
	require.NoError(t, err)
	err = dbase.InsertBlueprint(ctx, id, versionId, "000000", "000000", name, description, message, json.RawMessage(`{}`))
	require.NoError(t, err)

	tests := map[string]struct {
		payload any
		expect  int
	}{
		"empty targets":    {payload: strings.NewReader(""), expect: 3},
		"multiple targets": {payload: ComposeBlueprintJSONBody{ImageTypes: &[]ImageTypes{"aws", "guest-image", "gcp"}}, expect: 3},
		"one target":       {payload: ComposeBlueprintJSONBody{ImageTypes: &[]ImageTypes{"aws"}}, expect: 2},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			respStatusCode, body := tutils.PostResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s/compose", id.String()), tc.payload)
			require.Equal(t, http.StatusCreated, respStatusCode)

			var result []ComposeResponse
			err = json.Unmarshal([]byte(body), &result)
			require.NoError(t, err)
			require.Len(t, result, tc.expect)
			for i := 0; i < tc.expect; i++ {
				require.Equal(t, ids[len(ids)-tc.expect+i], result[i].Id)
			}
		})
	}
}

func TestHandlers_GetBlueprintComposes(t *testing.T) {
	ctx := context.Background()
	blueprintId := uuid.New()
	versionId := uuid.New()
	version2Id := uuid.New()
	imageName := "MyImageName"
	clientId := "ui"

	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	var result ComposesResponse

	err = dbase.InsertBlueprint(ctx, blueprintId, versionId, "000000", "500000", "blueprint", "blueprint desc", json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), json.RawMessage(`{}`))
	require.NoError(t, err)
	id1 := uuid.New()
	err = dbase.InsertCompose(ctx, id1, "500000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "edge-installer"}]}`), &clientId, &versionId)
	require.NoError(t, err)
	id2 := uuid.New()
	err = dbase.InsertCompose(ctx, id2, "500000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), &clientId, &versionId)
	require.NoError(t, err)

	err = dbase.UpdateBlueprint(ctx, version2Id, blueprintId, "000000", "blueprint", "desc2", json.RawMessage(`{"image_requests": [{"image_type": "aws"}, {"image_type": "gcp"}]}`))
	require.NoError(t, err)
	id3 := uuid.New()
	err = dbase.InsertCompose(ctx, id3, "500000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), &clientId, &version2Id)
	require.NoError(t, err)
	id4 := uuid.New()
	err = dbase.InsertCompose(ctx, id4, "500000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "gcp"}]}`), &clientId, &version2Id)
	require.NoError(t, err)

	respStatusCode, body := tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s/composes", blueprintId.String()), &tutils.AuthString0)
	require.NoError(t, err)

	require.Equal(t, 200, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Equal(t, blueprintId, *result.Data[0].BlueprintId)
	require.Equal(t, 2, *result.Data[0].BlueprintVersion)
	require.Equal(t, fmt.Sprintf("/api/image-builder/v1.0/composes?blueprint_id=%s&limit=100&offset=0", blueprintId.String()), result.Links.First)
	require.Equal(t, fmt.Sprintf("/api/image-builder/v1.0/composes?blueprint_id=%s&limit=100&offset=3", blueprintId.String()), result.Links.Last)
	require.Equal(t, 4, len(result.Data))
	require.Equal(t, 4, result.Meta.Count)

	// get composes for specific version
	respStatusCode, body = tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s/composes?blueprint_version=2", blueprintId.String()), &tutils.AuthString0)
	require.NoError(t, err)

	require.Equal(t, 200, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Equal(t, blueprintId, *result.Data[0].BlueprintId)
	require.Equal(t, 2, *result.Data[0].BlueprintVersion)
	require.Equal(t, fmt.Sprintf("/api/image-builder/v1.0/composes?blueprint_id=%s&blueprint_version=2&limit=100&offset=0", blueprintId.String()), result.Links.First)
	require.Equal(t, fmt.Sprintf("/api/image-builder/v1.0/composes?blueprint_id=%s&blueprint_version=2&limit=100&offset=1", blueprintId.String()), result.Links.Last)
	require.Equal(t, 2, len(result.Data))
	require.Equal(t, 2, result.Meta.Count)

	// get composes for latest version
	respStatusCode, body = tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s/composes?blueprint_version=-1", blueprintId.String()), &tutils.AuthString0)
	require.NoError(t, err)

	require.Equal(t, 200, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Equal(t, blueprintId, *result.Data[0].BlueprintId)
	require.Equal(t, 2, *result.Data[0].BlueprintVersion)

	// get composes for non-existing blueprint
	respStatusCode, _ = tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/experimental/blueprints/%s/composes?blueprint_version=1", uuid.New().String()), &tutils.AuthString0)
	require.Equal(t, 404, respStatusCode)

	// get composes for a blueprint that does not have any composes
	id5 := uuid.New()
	versionId2 := uuid.New()
	err = dbase.InsertBlueprint(ctx, id5, versionId2, "000000", "500000", "newBlueprint", "blueprint desc", json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), json.RawMessage(`{}`))
	require.NoError(t, err)
	respStatusCode, body = tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/experimental/blueprints/%s/composes?blueprint_version=1", id5), &tutils.AuthString0)
	require.Equal(t, 200, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Equal(t, 0, len(result.Data))
	require.Equal(t, 0, result.Meta.Count)
}

func TestHandlers_GetBlueprint(t *testing.T) {
	ctx := context.Background()
	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	id := uuid.New()
	versionId := uuid.New()

	uploadOptions := UploadRequest_Options{}
	err = uploadOptions.FromAWSUploadRequestOptions(AWSUploadRequestOptions{
		ShareWithAccounts: common.ToPtr([]string{"test-account"}),
	})
	require.NoError(t, err)
	name := "blueprint"
	description := "desc"
	blueprint := BlueprintBody{
		Customizations: Customizations{
			Packages: common.ToPtr([]string{"nginx"}),
		},
		Distribution: "centos-9",
		ImageRequests: []ImageRequest{
			{
				Architecture: ImageRequestArchitectureX8664,
				ImageType:    ImageTypesAws,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAws,
					Options: uploadOptions,
				},
			},
			{
				Architecture: ImageRequestArchitectureAarch64,
				ImageType:    ImageTypesAws,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAws,
					Options: uploadOptions,
				},
			},
		},
	}

	var message []byte
	message, err = json.Marshal(blueprint)
	require.NoError(t, err)
	err = dbase.InsertBlueprint(ctx, id, versionId, "000000", "000000", name, description, message, json.RawMessage(`{}`))
	require.NoError(t, err)

	respStatusCode, body := tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s", id.String()), &tutils.AuthString0)
	require.Equal(t, http.StatusOK, respStatusCode)

	var result BlueprintResponse
	require.Equal(t, 200, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Equal(t, id, result.Id)
	require.Equal(t, description, result.Description)
	require.Equal(t, name, result.Name)
	require.Equal(t, blueprint.ImageRequests, result.ImageRequests)
	require.Equal(t, blueprint.Distribution, result.Distribution)
	require.Equal(t, blueprint.Customizations, result.Customizations)

	respStatusCodeNotFound, _ := tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/experimental/blueprints/%s", uuid.New()), &tutils.AuthString0)
	require.Equal(t, http.StatusNotFound, respStatusCodeNotFound)
}

func TestHandlers_ExportBlueprint(t *testing.T) {
	ctx := context.Background()
	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	id := uuid.New()
	versionId := uuid.New()

	uploadOptions := UploadRequest_Options{}
	err = uploadOptions.FromAWSUploadRequestOptions(AWSUploadRequestOptions{
		ShareWithAccounts: common.ToPtr([]string{"test-account"}),
	})
	require.NoError(t, err)
	name := "blueprint"
	description := "desc"
	blueprint := BlueprintBody{
		Customizations: Customizations{
			Packages: common.ToPtr([]string{"nginx"}),
			Subscription: &Subscription{
				ActivationKey: "aaa",
			},
		},
		Distribution: "centos-9",
		ImageRequests: []ImageRequest{
			{
				Architecture: ImageRequestArchitectureX8664,
				ImageType:    ImageTypesAws,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAws,
					Options: uploadOptions,
				},
			},
			{
				Architecture: ImageRequestArchitectureAarch64,
				ImageType:    ImageTypesAws,
				UploadRequest: UploadRequest{
					Type:    UploadTypesAws,
					Options: uploadOptions,
				},
			},
		},
	}

	var message []byte
	message, err = json.Marshal(blueprint)
	require.NoError(t, err)

	parentId := uuid.New()
	exportedAt := time.RFC3339
	metadata := BlueprintMetadata{
		ParentId:   &parentId,
		ExportedAt: exportedAt,
	}
	var metadataMessage []byte
	metadataMessage, err = json.Marshal(metadata)
	require.NoError(t, err)

	err = dbase.InsertBlueprint(ctx, id, versionId, "000000", "000000", name, description, message, metadataMessage)
	require.NoError(t, err)

	respStatusCode, body := tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s/export", id.String()), &tutils.AuthString0)
	require.Equal(t, http.StatusOK, respStatusCode)

	var result BlueprintExportResponse
	require.Equal(t, 200, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Equal(t, description, result.Description)
	require.Equal(t, name, result.Name)
	require.Equal(t, blueprint.Distribution, result.Distribution)
	require.Equal(t, blueprint.Customizations.Packages, result.Customizations.Packages)
	require.Equal(t, &Subscription{}, result.Customizations.Subscription)
	require.Equal(t, &id, result.Metadata.ParentId)
	require.NotEqual(t, metadata.ExportedAt, result.Metadata.ExportedAt)
}

func TestHandlers_GetBlueprints(t *testing.T) {
	ctx := context.Background()
	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	blueprintId := uuid.New()
	versionId := uuid.New()
	err = dbase.InsertBlueprint(ctx, blueprintId, versionId, "000000", "000000", "blueprint", "blueprint desc", json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, err)
	blueprintId2 := uuid.New()
	versionId2 := uuid.New()
	err = dbase.InsertBlueprint(ctx, blueprintId2, versionId2, "000000", "000000", "Blueprint2", "blueprint desc", json.RawMessage(`{}`), json.RawMessage(`{}`))
	require.NoError(t, err)

	var result BlueprintsResponse
	respStatusCode, body := tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/blueprints?name=blueprint", &tutils.AuthString0)
	require.Equal(t, http.StatusOK, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Len(t, result.Data, 1)
	require.Equal(t, blueprintId, result.Data[0].Id)

	respStatusCode, body = tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/blueprints?name=Blueprint", &tutils.AuthString0)
	require.Equal(t, http.StatusOK, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Len(t, result.Data, 0)
}

func TestHandlers_DeleteBlueprint(t *testing.T) {
	ctx := context.Background()
	blueprintId := uuid.New()
	versionId := uuid.New()
	version2Id := uuid.New()
	clientId := "ui"
	imageName := "MyImageName"

	dbase, err := dbc.NewDB()
	require.NoError(t, err)

	db_srv, tokenSrv := startServer(t, &testServerClientsConf{}, &ServerConfig{
		DBase:            dbase,
		DistributionsDir: "../../distributions",
	})
	defer func() {
		err := db_srv.Shutdown(ctx)
		require.NoError(t, err)
	}()
	defer tokenSrv.Close()

	blueprintName := "blueprint"
	err = dbase.InsertBlueprint(ctx, blueprintId, versionId, "000000", "000000", blueprintName, "blueprint desc", json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), json.RawMessage(`{}`))
	require.NoError(t, err)
	id1 := uuid.New()
	err = dbase.InsertCompose(ctx, id1, "000000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "edge-installer"}]}`), &clientId, &versionId)
	require.NoError(t, err)

	id2 := uuid.New()
	err = dbase.InsertCompose(ctx, id2, "000000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), &clientId, &versionId)
	require.NoError(t, err)

	err = dbase.UpdateBlueprint(ctx, version2Id, blueprintId, "000000", "blueprint", "desc2", json.RawMessage(`{"image_requests": [{"image_type": "aws"}, {"image_type": "gcp"}]}`))
	require.NoError(t, err)
	id3 := uuid.New()
	err = dbase.InsertCompose(ctx, id3, "000000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), &clientId, &version2Id)
	require.NoError(t, err)
	id4 := uuid.New()
	err = dbase.InsertCompose(ctx, id4, "000000", "user100000@test.test", "000000", &imageName, json.RawMessage(`{"image_requests": [{"image_type": "gcp"}]}`), &clientId, &version2Id)
	require.NoError(t, err)

	respStatusCode, body := tutils.DeleteResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s", blueprintId.String()))
	require.Equal(t, 204, respStatusCode)
	require.Equal(t, "", body)

	var errorResponse HTTPErrorList
	notFoundCode, body := tutils.DeleteResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s", blueprintId.String()))
	require.Equal(t, 404, notFoundCode)
	err = json.Unmarshal([]byte(body), &errorResponse)
	require.NoError(t, err)
	require.Equal(t, "Not Found", errorResponse.Errors[0].Detail)

	_, err = dbase.GetBlueprint(ctx, blueprintId, "000000")
	require.ErrorIs(t, err, db.BlueprintNotFoundError)

	// We should not be able to list deleted blueprint
	var result BlueprintsResponse
	respStatusCode, body = tutils.GetResponseBody(t, "http://localhost:8086/api/image-builder/v1/experimental/blueprints?name=blueprint", &tutils.AuthString0)
	require.Equal(t, http.StatusOK, respStatusCode)
	err = json.Unmarshal([]byte(body), &result)
	require.NoError(t, err)
	require.Len(t, result.Data, 0)

	// We should not be able to update deleted blueprint
	id5 := uuid.New()
	err = dbase.UpdateBlueprint(ctx, id5, blueprintId, "000000", "newName", "desc2", json.RawMessage(`{"image_requests": [{"image_type": "aws"}, {"image_type": "gcp"}]}`))
	require.ErrorIs(t, err, db.BlueprintNotFoundError)

	// Composes should not be assigned to the blueprint anymore
	respStatusCode, _ = tutils.GetResponseBody(t, fmt.Sprintf("http://localhost:8086/api/image-builder/v1/blueprints/%s/composes", blueprintId.String()), &tutils.AuthString0)
	require.Equal(t, 404, respStatusCode)

	// We should be able to create a Blueprint with same name
	blueprintId2 := uuid.New()
	versionId2 := uuid.New()
	err = dbase.InsertBlueprint(ctx, blueprintId2, versionId2, "000000", "000000", blueprintName, "blueprint desc", json.RawMessage(`{"image_requests": [{"image_type": "aws"}]}`), json.RawMessage(`{}`))
	require.NoError(t, err)

	bpComposes, err := dbase.GetBlueprintComposes(ctx, "000000", blueprintId2, nil, (time.Hour * 24 * 14), 10, 0, nil)
	require.Len(t, bpComposes, 0)
	require.NoError(t, err)
}
