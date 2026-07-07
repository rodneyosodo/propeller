package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/absmach/propeller"
	pkgerrors "github.com/absmach/propeller/pkg/errors"
	kithttp "github.com/go-kit/kit/transport/http"
)

var (
	_ = propeller.Version
	_ = propeller.Commit
	_ = propeller.BuildTime
)

const (
	OffsetKey   = "offset"
	LimitKey    = "limit"
	MetadataKey = "metadata"
	DefOffset   = 0
	DefLimit    = 100

	ContentType = "application/json"

	MaxLimitSize = 100
)

var (
	ErrValidation             = errors.New("something went wrong with the request")
	ErrMissingName            = errors.New("missing identity name")
	ErrMissingID              = errors.New("missing entity id")
	ErrInvalidQueryParams     = errors.New("invalid query parameters")
	ErrLimitSize              = errors.New("invalid limit size")
	ErrUnsupportedContentType = errors.New("unsupported content type")
)

type Response interface {
	Code() int
	Headers() map[string]string
	Empty() bool
}

func EncodeResponse(_ context.Context, w http.ResponseWriter, response any) error {
	if ar, ok := response.(Response); ok {
		for k, v := range ar.Headers() {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", ContentType)
		w.WriteHeader(ar.Code())

		if ar.Empty() {
			return nil
		}
	}

	return json.NewEncoder(w).Encode(response)
}

func EncodeError(_ context.Context, err error, w http.ResponseWriter) {
	w.Header().Set("Content-Type", ContentType)
	switch {
	case errors.Is(err, ErrValidation),
		errors.Is(err, pkgerrors.ErrEmptyKey),
		errors.Is(err, pkgerrors.ErrInvalidValue):
		w.WriteHeader(http.StatusBadRequest)
	case errors.Is(err, pkgerrors.ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	case errors.Is(err, pkgerrors.ErrConflict):
		w.WriteHeader(http.StatusConflict)
	default:
		w.WriteHeader(http.StatusInternalServerError)
	}

	if err := json.NewEncoder(w).Encode(err); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func LoggingErrorEncoder(logger *slog.Logger, enc kithttp.ErrorEncoder) kithttp.ErrorEncoder {
	return func(ctx context.Context, err error, w http.ResponseWriter) {
		if errors.Is(err, ErrValidation) {
			logger.Error(err.Error())
		}
		enc(ctx, err, w)
	}
}

func ReadStringQuery(r *http.Request, key, def string) (string, error) {
	vals := r.URL.Query()[key]
	if len(vals) == 0 {
		return def, nil
	}

	return vals[0], nil
}

func ReadMetadataQuery(r *http.Request, key string, def map[string]any) (map[string]any, error) {
	vals := r.URL.Query()[key]
	if len(vals) == 0 {
		return def, nil
	}
	if len(vals) > 1 {
		return nil, errors.New("multiple values not supported for metadata")
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(vals[0]), &meta); err != nil {
		return nil, fmt.Errorf("invalid metadata: %w", err)
	}

	return meta, nil
}

func HealthHandler(service, instanceID string) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", ContentType)
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}
		w.WriteHeader(http.StatusOK)
		err := json.NewEncoder(w).Encode(propeller.HealthInfo{
			Status:      "pass",
			Version:     propeller.Version,
			Commit:      propeller.Commit,
			Description: service + " service",
			BuildTime:   propeller.BuildTime,
			InstanceID:  instanceID,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)

			return
		}
	})
}

type number interface {
	int64 | float64 | uint16 | uint64
}

func ReadNumQuery[N number](r *http.Request, key string, def N) (N, error) {
	vals := r.URL.Query()[key]
	if len(vals) == 0 {
		return def, nil
	}
	raw := vals[0]
	var v N
	switch any(v).(type) {
	case int64:
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return def, fmt.Errorf("invalid %s: %w", key, err)
		}

		return N(n), nil
	case float64:
		n, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return def, fmt.Errorf("invalid %s: %w", key, err)
		}

		return N(n), nil
	case uint16:
		n, err := strconv.ParseUint(raw, 10, 16)
		if err != nil {
			return def, fmt.Errorf("invalid %s: %w", key, err)
		}

		return N(n), nil
	case uint64:
		n, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return def, fmt.Errorf("invalid %s: %w", key, err)
		}

		return N(n), nil
	default:
		return def, errors.New("unsupported numeric type")
	}
}
