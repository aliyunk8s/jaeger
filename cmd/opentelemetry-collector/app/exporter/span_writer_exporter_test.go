// Copyright (c) 2020 The Jaeger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"context"
	"errors"
	"testing"

	tracev1 "github.com/open-telemetry/opentelemetry-proto/gen/go/trace/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/uber/jaeger-lib/metrics"
	"go.opentelemetry.io/collector/config/configmodels"
	"go.opentelemetry.io/collector/consumer/pdata"
	"go.uber.org/zap"

	"github.com/jaegertracing/jaeger/model"
	"github.com/jaegertracing/jaeger/storage/dependencystore"
	"github.com/jaegertracing/jaeger/storage/spanstore"
)

func TestNew_closableWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, mockStorageFactory{spanWriter: spanWriter{}})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown(context.Background()))
}

func TestNew_noClosableWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, mockStorageFactory{spanWriter: noClosableWriter{}})
	require.NoError(t, err)
	assert.NotNil(t, exporter)
	assert.Nil(t, exporter.Shutdown(context.Background()))
}

func TestNew_failedToCreateWriter(t *testing.T) {
	exporter, err := NewSpanWriterExporter(&configmodels.ExporterSettings{}, mockStorageFactory{err: errors.New("failed to create writer"), spanWriter: spanWriter{}})
	require.Nil(t, exporter)
	assert.Error(t, err, "failed to create writer")
}

func TestStore(t *testing.T) {
	traceID := []byte("0123456789abcdef")
	spanID := []byte("01234567")
	tests := []struct {
		storage storage
		data    pdata.Traces
		err     string
		dropped int
		caption string
	}{
		{
			caption: "nothing to store",
			storage: storage{Writer: spanWriter{}},
			data:    pdata.TracesFromOtlp([]*tracev1.ResourceSpans{}),
			dropped: 0,
		},
		{
			caption: "wrong data",
			storage: storage{Writer: spanWriter{}},
			data:    pdata.TracesFromOtlp([]*tracev1.ResourceSpans{{InstrumentationLibrarySpans: []*tracev1.InstrumentationLibrarySpans{{Spans: []*tracev1.Span{{}}}}}}),
			err:     "TraceID is nil",
			dropped: 1,
		},
		{
			caption: "one error in writer",
			storage: storage{Writer: spanWriter{err: errors.New("could not store")}},
			data: pdata.TracesFromOtlp([]*tracev1.ResourceSpans{{
				InstrumentationLibrarySpans: []*tracev1.InstrumentationLibrarySpans{{
					Spans: []*tracev1.Span{
						{TraceId: traceID, SpanId: spanID, Name: "error"},
						{TraceId: traceID, SpanId: spanID},
					},
				}},
			}}),
			dropped: 1,
			err:     "could not store",
		},
		{
			caption: "two errors in writer",
			storage: storage{Writer: spanWriter{err: errors.New("could not store")}},
			data: pdata.TracesFromOtlp([]*tracev1.ResourceSpans{{
				InstrumentationLibrarySpans: []*tracev1.InstrumentationLibrarySpans{{
					Spans: []*tracev1.Span{
						{TraceId: traceID, SpanId: spanID, Name: "error"},
						{TraceId: traceID, SpanId: spanID, Name: "error"},
					},
				}},
			}}),
			dropped: 2,
			err:     "[could not store; could not store]",
		},
	}
	for _, test := range tests {
		t.Run(test.caption, func(t *testing.T) {
			dropped, err := test.storage.traceDataPusher(context.Background(), test.data)
			assert.Equal(t, test.dropped, dropped)
			if test.err != "" {
				assert.Contains(t, err.Error(), test.err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

type spanWriter struct {
	err error
}

func (w spanWriter) WriteSpan(span *model.Span) error {
	if span.GetOperationName() == "error" {
		return w.err
	}
	return nil
}

func (spanWriter) Close() error {
	return nil
}

type noClosableWriter struct {
}

func (noClosableWriter) WriteSpan(span *model.Span) error {
	return nil
}

type mockStorageFactory struct {
	err        error
	spanWriter spanstore.Writer
}

func (m mockStorageFactory) CreateSpanWriter() (spanstore.Writer, error) {
	return m.spanWriter, m.err
}
func (mockStorageFactory) CreateSpanReader() (spanstore.Reader, error) {
	return nil, nil
}
func (mockStorageFactory) CreateDependencyReader() (dependencystore.Reader, error) {
	return nil, nil
}
func (mockStorageFactory) Initialize(metrics.Factory, *zap.Logger) error {
	return nil
}
