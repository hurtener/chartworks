import {mapCatalog} from './mapping-fixture.mjs';
// Deterministic synthetic host, never a Pengui deployment or live-provider proof.
// The retained shapes below are copied from TestReportAppCanvas Delivery.View
// results. Identifiers/times and visible labels are synthetic fixture coordinates.
const capturedCommon = {
  "version": "reporting-view-v1",
  "summary": {
    "kind": "report",
    "run": "synthetic-domain-run",
    "target": {
      "kind": "report",
      "id": "canvas-report",
      "revision": 2
    },
    "state": "completed",
    "code": "",
    "private": true,
    "created_at": "2026-10-03T12:00:00Z",
    "expires_at": "2099-01-01T00:00:00Z"
  },
  "locale": "en-US",
  "timezone": "UTC",
  "pages": [
    {
      "locale": "en-US",
      "timezone": "UTC",
      "id": "main",
      "report": "canvas-report",
      "revision": 2,
      "title": "Approved revenue report",
      "widgets": [
        {
          "id": "intro",
          "kind": "text",
          "state": "completed",
          "grid": {
            "column": 0,
            "row": 0,
            "width": 12,
            "height": 1
          },
          "presentation": {},
          "outputs": [],
          "parameters": []
        },
        {
          "selection": {
            "definition_version": 1,
            "version": 2,
            "mode": "explicit",
            "requested": [
              "kpi-main"
            ],
            "selected": [
              "kpi-main"
            ],
            "choices": [
              {
                "id": "kpi-main",
                "kind": "kpi",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Indicator · kpi-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Indicador · kpi-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 0
                },
                "selected": true,
                "state": "selected"
              },
              {
                "id": "trend-main",
                "kind": "chart",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Chart · trend-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Gráfico · trend-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 1
                },
                "selected": false,
                "state": "omitted",
                "code": "not_selected"
              },
              {
                "id": "table-main",
                "kind": "table",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Table · table-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Tabla · table-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 2
                },
                "selected": false,
                "state": "omitted",
                "code": "not_selected"
              }
            ]
          },
          "query_limits": {
            "max_rows": 1000,
            "max_bytes": 1048576,
            "timeout_ms": 60000,
            "query_attempts": 3
          },
          "id": "kpi",
          "kind": "block",
          "state": "completed",
          "grid": {
            "column": 0,
            "row": 1,
            "width": 3,
            "height": 2
          },
          "presentation": {},
          "outputs": [
            "kpi-main"
          ],
          "parameters": [],
          "trust": {
            "publication": "published",
            "certification": "none",
            "health": {
              "status": "healthy",
              "observed_at": "2026-10-03T12:00:00Z",
              "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
              "reason": "validated_observation"
            }
          },
          "observed_at": "2026-10-03T12:00:00Z"
        },
        {
          "selection": {
            "definition_version": 1,
            "version": 2,
            "mode": "explicit",
            "requested": [
              "trend-main"
            ],
            "selected": [
              "trend-main"
            ],
            "choices": [
              {
                "id": "kpi-main",
                "kind": "kpi",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Indicator · kpi-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Indicador · kpi-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 0
                },
                "selected": false,
                "state": "omitted",
                "code": "not_selected"
              },
              {
                "id": "trend-main",
                "kind": "chart",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Chart · trend-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Gráfico · trend-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 1
                },
                "selected": true,
                "state": "selected"
              },
              {
                "id": "table-main",
                "kind": "table",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Table · table-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Tabla · table-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 2
                },
                "selected": false,
                "state": "omitted",
                "code": "not_selected"
              }
            ]
          },
          "query_limits": {
            "max_rows": 1000,
            "max_bytes": 1048576,
            "timeout_ms": 60000,
            "query_attempts": 3
          },
          "id": "trend",
          "kind": "block",
          "state": "completed",
          "grid": {
            "column": 3,
            "row": 1,
            "width": 9,
            "height": 2
          },
          "presentation": {
            "title": "Revenue trend",
            "subtitle": "Approved source values",
            "density": "comfortable"
          },
          "outputs": [
            "trend-main"
          ],
          "parameters": [],
          "trust": {
            "publication": "published",
            "certification": "none",
            "health": {
              "status": "healthy",
              "observed_at": "2026-10-03T12:00:00Z",
              "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
              "reason": "validated_observation"
            }
          },
          "observed_at": "2026-10-03T12:00:00Z"
        },
        {
          "selection": {
            "definition_version": 1,
            "version": 2,
            "mode": "explicit",
            "requested": [
              "table-main"
            ],
            "selected": [
              "table-main"
            ],
            "choices": [
              {
                "id": "kpi-main",
                "kind": "kpi",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Indicator · kpi-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Indicador · kpi-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 0
                },
                "selected": false,
                "state": "omitted",
                "code": "not_selected"
              },
              {
                "id": "trend-main",
                "kind": "chart",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Chart · trend-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Gráfico · trend-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 1
                },
                "selected": false,
                "state": "omitted",
                "code": "not_selected"
              },
              {
                "id": "table-main",
                "kind": "table",
                "intent": {
                  "metadata": [
                    {
                      "locale": "en-US",
                      "display_name": "Revenue evidence — Table · table-main",
                      "description": "Synthetic governed output"
                    },
                    {
                      "locale": "es-AR",
                      "display_name": "Ingresos — Tabla · table-main",
                      "description": "Datos sintéticos"
                    }
                  ],
                  "enabled": true,
                  "default_selected": true,
                  "display_order": 2
                },
                "selected": true,
                "state": "selected"
              }
            ]
          },
          "query_limits": {
            "max_rows": 1000,
            "max_bytes": 1048576,
            "timeout_ms": 60000,
            "query_attempts": 3
          },
          "id": "table",
          "kind": "block",
          "state": "completed",
          "grid": {
            "column": 0,
            "row": 3,
            "width": 12,
            "height": 3
          },
          "presentation": {},
          "outputs": [
            "table-main"
          ],
          "parameters": [],
          "trust": {
            "publication": "published",
            "certification": "none",
            "health": {
              "status": "healthy",
              "observed_at": "2026-10-03T12:00:00Z",
              "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
              "reason": "validated_observation"
            }
          },
          "observed_at": "2026-10-03T12:00:00Z"
        }
      ]
    }
  ],
  "filters": [],
  "mixed_freshness": false,
  "redacted": false
};

export const capturedViews = {
  heading: {...capturedCommon, ...{
    "selection": {
      "kind": "report",
      "run": "synthetic-domain-run",
      "page": "main",
      "widget": "intro",
      "output": "",
      "offset": 0,
      "limit": 1
    },
    "outputs": [],
    "text": {
      "format": "markdown",
      "text": "# Revenue report\n\nApproved retained evidence"
    },
    "page_bounds": {
      "offset": 0,
      "limit": 1,
      "total": 0
    }
  }},
  kpi: {...capturedCommon, ...{
    "accepted_selection": {
      "definition_version": 1,
      "version": 2,
      "mode": "explicit",
      "requested": [
        "kpi-main"
      ],
      "selected": [
        "kpi-main"
      ],
      "choices": [
        {
          "id": "kpi-main",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Indicator · kpi-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Indicador · kpi-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "selected": true,
          "state": "selected"
        },
        {
          "id": "trend-main",
          "kind": "chart",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Chart · trend-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Gráfico · trend-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 1
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        },
        {
          "id": "table-main",
          "kind": "table",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Table · table-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Tabla · table-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 2
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        }
      ]
    },
    "query_limits": {
      "max_rows": 1000,
      "max_bytes": 1048576,
      "timeout_ms": 60000,
      "query_attempts": 3
    },
    "selection": {
      "kind": "report",
      "run": "synthetic-domain-run",
      "page": "main",
      "widget": "kpi",
      "output": "kpi-main",
      "offset": 0,
      "limit": 1
    },
    "outputs": [
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Indicator · kpi-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Indicador · kpi-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 0,
        "enabled": true,
        "default_selected": true,
        "selected": true,
        "state": "selected",
        "id": "kpi-main",
        "kind": "kpi",
        "title": "Revenue evidence — Indicator · kpi-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Chart · trend-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Gráfico · trend-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 1,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "trend-main",
        "kind": "chart",
        "title": "Revenue evidence — Chart · trend-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Table · table-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Tabla · table-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 2,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "table-main",
        "kind": "table",
        "title": "Revenue evidence — Table · table-main"
      }
    ],
    "trust": {
      "publication": "published",
      "certification": "none",
      "health": {
        "status": "healthy",
        "observed_at": "2026-10-03T12:00:00Z",
        "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "reason": "validated_observation"
      }
    },
    "observed_at": "2026-10-03T12:00:00Z",
    "output": {
      "result_policy": [
        {
          "field": "sale_date",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        },
        {
          "field": "amount",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        }
      ],
      "intent": {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Indicator · kpi-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Indicador · kpi-main",
            "description": "Datos sintéticos"
          }
        ],
        "enabled": true,
        "default_selected": true,
        "display_order": 0
      },
      "id": "kpi-main",
      "kind": "kpi",
      "state": "succeeded",
      "code": "",
      "retained_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "chart": {
        "version": 3,
        "kind": "kpi",
        "mapping": {
          "version": 3,
          "kind": "kpi",
          "columns": [
            {
              "id": "c0",
              "name": "sale_date",
              "display_label": "Sale date",
              "type": "temporal",
              "role": "time",
              "grain": "",
              "aggregation": "",
              "format": {
                "unit": "",
                "currency": "",
                "percent": "",
                "fraction_digits": 0,
                "locale": "en-US",
                "date_pattern": "date_medium"
              },
              "provenance": {
                "version": 1,
                "source": "",
                "source_revision": 0,
                "topic": "",
                "topic_version": "",
                "semantic_id": ""
              }
            },
            {
              "id": "c1",
              "name": "amount",
              "display_label": "Revenue",
              "type": "decimal",
              "role": "measure",
              "grain": "",
              "aggregation": "sum",
              "format": {
                "unit": "revenue",
                "currency": "USD",
                "percent": "",
                "fraction_digits": 3,
                "locale": "en-US"
              },
              "provenance": {
                "version": 1,
                "source": "",
                "source_revision": 0,
                "topic": "",
                "topic_version": "",
                "semantic_id": ""
              }
            }
          ],
          "bindings": {
            "category": "c0",
            "value": "c1"
          },
          "order": [
            {
              "column": "c0",
              "direction": "asc"
            }
          ],
          "options": {
            "title": "",
            "legend": {
              "visible": true,
              "position": "bottom"
            },
            "label_max_runes": 80
          },
          "kpi": {
            "value_row": "first",
            "comparison_mode": "none",
            "show_delta": false,
            "show_percent_delta": false,
            "show_target_difference": false,
            "sparkline": true,
            "thresholds": []
          }
        },
        "columns": [
          {
            "id": "c0",
            "name": "sale_date",
            "display_label": "Sale date",
            "type": "temporal",
            "role": "time",
            "grain": "",
            "aggregation": "",
            "format": {
              "unit": "",
              "currency": "",
              "percent": "",
              "fraction_digits": 0,
              "locale": "en-US",
              "date_pattern": "date_medium"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          },
          {
            "id": "c1",
            "name": "amount",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "revenue",
              "currency": "USD",
              "percent": "",
              "fraction_digits": 3,
              "locale": "en-US"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          }
        ],
        "rows": [],
        "points": [
          {
            "row": 0,
            "category": {
              "null": false,
              "value": ""
            },
            "series": {
              "null": false,
              "value": ""
            },
            "parent": {
              "null": false,
              "value": ""
            },
            "x": {
              "null": false,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "y": {
              "null": false,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "value": {
              "null": false,
              "exact": "9007199254740993.125",
              "coordinate": 9007199254740994,
              "approximate": true
            }
          }
        ],
        "totals": [],
        "state": "ready",
        "input_rows": 2,
        "omitted_rows": 0,
        "completeness": {
          "status": "complete_result",
          "reason": ""
        },
        "warnings": [],
        "kpi_result": {
          "value": {
            "null": false,
            "exact": "9007199254740993.125",
            "coordinate": 9007199254740994,
            "approximate": true
          },
          "sparkline": [
            {
              "null": false,
              "exact": "9007199254740993.125",
              "coordinate": 9007199254740994,
              "approximate": true
            },
            {
              "null": false,
              "exact": "5.500",
              "coordinate": 5.5,
              "approximate": false
            }
          ]
        }
      }
    },
    "page_bounds": {
      "offset": 0,
      "limit": 1,
      "total": 1
    }
  }},
  trend: {...capturedCommon, ...{
    "accepted_selection": {
      "definition_version": 1,
      "version": 2,
      "mode": "explicit",
      "requested": [
        "trend-main"
      ],
      "selected": [
        "trend-main"
      ],
      "choices": [
        {
          "id": "kpi-main",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Indicator · kpi-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Indicador · kpi-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        },
        {
          "id": "trend-main",
          "kind": "chart",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Chart · trend-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Gráfico · trend-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 1
          },
          "selected": true,
          "state": "selected"
        },
        {
          "id": "table-main",
          "kind": "table",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Table · table-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Tabla · table-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 2
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        }
      ]
    },
    "query_limits": {
      "max_rows": 1000,
      "max_bytes": 1048576,
      "timeout_ms": 60000,
      "query_attempts": 3
    },
    "selection": {
      "kind": "report",
      "run": "synthetic-domain-run",
      "page": "main",
      "widget": "trend",
      "output": "trend-main",
      "offset": 0,
      "limit": 1
    },
    "outputs": [
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Indicator · kpi-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Indicador · kpi-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 0,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "kpi-main",
        "kind": "kpi",
        "title": "Revenue evidence — Indicator · kpi-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Chart · trend-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Gráfico · trend-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 1,
        "enabled": true,
        "default_selected": true,
        "selected": true,
        "state": "selected",
        "id": "trend-main",
        "kind": "chart",
        "title": "Revenue evidence — Chart · trend-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Table · table-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Tabla · table-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 2,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "table-main",
        "kind": "table",
        "title": "Revenue evidence — Table · table-main"
      }
    ],
    "trust": {
      "publication": "published",
      "certification": "none",
      "health": {
        "status": "healthy",
        "observed_at": "2026-10-03T12:00:00Z",
        "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "reason": "validated_observation"
      }
    },
    "observed_at": "2026-10-03T12:00:00Z",
    "output": {
      "result_policy": [
        {
          "field": "sale_date",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        },
        {
          "field": "amount",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        }
      ],
      "intent": {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Chart · trend-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Gráfico · trend-main",
            "description": "Datos sintéticos"
          }
        ],
        "enabled": true,
        "default_selected": true,
        "display_order": 1
      },
      "id": "trend-main",
      "kind": "chart",
      "state": "succeeded",
      "code": "",
      "retained_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "chart": {
        "version": 1,
        "kind": "line",
        "mapping": {
          "version": 1,
          "kind": "line",
          "columns": [
            {
              "id": "c0",
              "name": "sale_date",
              "display_label": "Sale date",
              "type": "temporal",
              "role": "time",
              "grain": "",
              "aggregation": "",
              "format": {
                "unit": "",
                "currency": "",
                "percent": "",
                "fraction_digits": 0,
                "locale": "en-US",
                "date_pattern": "date_medium"
              },
              "provenance": {
                "version": 1,
                "source": "",
                "source_revision": 0,
                "topic": "",
                "topic_version": "",
                "semantic_id": ""
              }
            },
            {
              "id": "c1",
              "name": "amount",
              "display_label": "Revenue",
              "type": "decimal",
              "role": "measure",
              "grain": "",
              "aggregation": "sum",
              "format": {
                "unit": "revenue",
                "currency": "USD",
                "percent": "",
                "fraction_digits": 3,
                "locale": "en-US"
              },
              "provenance": {
                "version": 1,
                "source": "",
                "source_revision": 0,
                "topic": "",
                "topic_version": "",
                "semantic_id": ""
              }
            }
          ],
          "bindings": {
            "category": "c0",
            "value": "c1"
          },
          "order": [
            {
              "column": "c0",
              "direction": "asc"
            }
          ],
          "options": {
            "title": "",
            "legend": {
              "visible": true,
              "position": "bottom"
            },
            "label_max_runes": 80
          }
        },
        "columns": [
          {
            "id": "c0",
            "name": "sale_date",
            "display_label": "Sale date",
            "type": "temporal",
            "role": "time",
            "grain": "",
            "aggregation": "",
            "format": {
              "unit": "",
              "currency": "",
              "percent": "",
              "fraction_digits": 0,
              "locale": "en-US",
              "date_pattern": "date_medium"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          },
          {
            "id": "c1",
            "name": "amount",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "revenue",
              "currency": "USD",
              "percent": "",
              "fraction_digits": 3,
              "locale": "en-US"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          }
        ],
        "rows": [],
        "points": [
          {
            "row": 0,
            "category": {
              "null": false,
              "value": "2026-01-02"
            },
            "series": {
              "null": true,
              "value": ""
            },
            "parent": {
              "null": true,
              "value": ""
            },
            "x": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "y": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "value": {
              "null": false,
              "exact": "9007199254740993.125",
              "coordinate": 9007199254740994,
              "approximate": true
            }
          },
          {
            "row": 1,
            "category": {
              "null": false,
              "value": "2026-01-03"
            },
            "series": {
              "null": true,
              "value": ""
            },
            "parent": {
              "null": true,
              "value": ""
            },
            "x": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "y": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "value": {
              "null": false,
              "exact": "5.500",
              "coordinate": 5.5,
              "approximate": false
            }
          }
        ],
        "totals": [
          {
            "column": "c1",
            "value": {
              "null": false,
              "value": "9007199254740998.625"
            },
            "scope": "complete_result"
          }
        ],
        "state": "ready",
        "input_rows": 2,
        "omitted_rows": 0,
        "completeness": {
          "status": "complete_result",
          "reason": ""
        },
        "warnings": [
          "geometry_approximate_labels_exact"
        ]
      }
    },
    "page_bounds": {
      "offset": 0,
      "limit": 1,
      "total": 2
    }
  }},
  table: {...capturedCommon, ...{
    "accepted_selection": {
      "definition_version": 1,
      "version": 2,
      "mode": "explicit",
      "requested": [
        "table-main"
      ],
      "selected": [
        "table-main"
      ],
      "choices": [
        {
          "id": "kpi-main",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Indicator · kpi-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Indicador · kpi-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        },
        {
          "id": "trend-main",
          "kind": "chart",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Chart · trend-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Gráfico · trend-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 1
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        },
        {
          "id": "table-main",
          "kind": "table",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Table · table-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Tabla · table-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 2
          },
          "selected": true,
          "state": "selected"
        }
      ]
    },
    "query_limits": {
      "max_rows": 1000,
      "max_bytes": 1048576,
      "timeout_ms": 60000,
      "query_attempts": 3
    },
    "selection": {
      "kind": "report",
      "run": "synthetic-domain-run",
      "page": "main",
      "widget": "table",
      "output": "table-main",
      "offset": 0,
      "limit": 1
    },
    "outputs": [
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Indicator · kpi-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Indicador · kpi-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 0,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "kpi-main",
        "kind": "kpi",
        "title": "Revenue evidence — Indicator · kpi-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Chart · trend-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Gráfico · trend-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 1,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "trend-main",
        "kind": "chart",
        "title": "Revenue evidence — Chart · trend-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Table · table-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Tabla · table-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 2,
        "enabled": true,
        "default_selected": true,
        "selected": true,
        "state": "selected",
        "id": "table-main",
        "kind": "table",
        "title": "Revenue evidence — Table · table-main"
      }
    ],
    "trust": {
      "publication": "published",
      "certification": "none",
      "health": {
        "status": "healthy",
        "observed_at": "2026-10-03T12:00:00Z",
        "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "reason": "validated_observation"
      }
    },
    "observed_at": "2026-10-03T12:00:00Z",
    "output": {
      "result_policy": [
        {
          "field": "sale_date",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        },
        {
          "field": "amount",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        }
      ],
      "intent": {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Table · table-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Tabla · table-main",
            "description": "Datos sintéticos"
          }
        ],
        "enabled": true,
        "default_selected": true,
        "display_order": 2
      },
      "id": "table-main",
      "kind": "table",
      "state": "succeeded",
      "code": "",
      "retained_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "table": {
        "columns": [
          {
            "id": "c0",
            "name": "sale_date",
            "display_label": "Sale date",
            "type": "temporal",
            "role": "time",
            "grain": "",
            "aggregation": "",
            "format": {
              "unit": "",
              "currency": "",
              "percent": "",
              "fraction_digits": 0,
              "locale": "en-US",
              "date_pattern": "date_medium"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          },
          {
            "id": "c1",
            "name": "amount",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "revenue",
              "currency": "USD",
              "percent": "",
              "fraction_digits": 3,
              "locale": "en-US"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          }
        ],
        "rows": [
          [
            {
              "null": false,
              "value": "2026-01-02"
            },
            {
              "null": false,
              "value": "9007199254740993.125"
            }
          ]
        ],
        "totals": [
          {
            "column": "c1",
            "value": {
              "null": false,
              "value": "9007199254740998.625"
            },
            "scope": "complete_result"
          }
        ],
        "completeness": {
          "status": "complete_result",
          "reason": ""
        },
        "warnings": []
      }
    },
    "page_bounds": {
      "offset": 0,
      "limit": 1,
      "total": 2,
      "next": 1
    }
  }},
  tableNext: {...capturedCommon, ...{
    "accepted_selection": {
      "definition_version": 1,
      "version": 2,
      "mode": "explicit",
      "requested": [
        "table-main"
      ],
      "selected": [
        "table-main"
      ],
      "choices": [
        {
          "id": "kpi-main",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Indicator · kpi-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Indicador · kpi-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        },
        {
          "id": "trend-main",
          "kind": "chart",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Chart · trend-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Gráfico · trend-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 1
          },
          "selected": false,
          "state": "omitted",
          "code": "not_selected"
        },
        {
          "id": "table-main",
          "kind": "table",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Revenue evidence — Table · table-main",
                "description": "Synthetic governed output"
              },
              {
                "locale": "es-AR",
                "display_name": "Ingresos — Tabla · table-main",
                "description": "Datos sintéticos"
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 2
          },
          "selected": true,
          "state": "selected"
        }
      ]
    },
    "query_limits": {
      "max_rows": 1000,
      "max_bytes": 1048576,
      "timeout_ms": 60000,
      "query_attempts": 3
    },
    "selection": {
      "kind": "report",
      "run": "synthetic-domain-run",
      "page": "main",
      "widget": "table",
      "output": "table-main",
      "offset": 1,
      "limit": 1
    },
    "outputs": [
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Indicator · kpi-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Indicador · kpi-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 0,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "kpi-main",
        "kind": "kpi",
        "title": "Revenue evidence — Indicator · kpi-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Chart · trend-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Gráfico · trend-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 1,
        "enabled": true,
        "default_selected": true,
        "selected": false,
        "state": "omitted",
        "code": "not_selected",
        "id": "trend-main",
        "kind": "chart",
        "title": "Revenue evidence — Chart · trend-main"
      },
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Table · table-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Tabla · table-main",
            "description": "Datos sintéticos"
          }
        ],
        "description": "Synthetic governed output",
        "locale": "en-US",
        "display_order": 2,
        "enabled": true,
        "default_selected": true,
        "selected": true,
        "state": "selected",
        "id": "table-main",
        "kind": "table",
        "title": "Revenue evidence — Table · table-main"
      }
    ],
    "trust": {
      "publication": "published",
      "certification": "none",
      "health": {
        "status": "healthy",
        "observed_at": "2026-10-03T12:00:00Z",
        "dependency_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
        "reason": "validated_observation"
      }
    },
    "observed_at": "2026-10-03T12:00:00Z",
    "output": {
      "result_policy": [
        {
          "field": "sale_date",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        },
        {
          "field": "amount",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
        }
      ],
      "intent": {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Revenue evidence — Table · table-main",
            "description": "Synthetic governed output"
          },
          {
            "locale": "es-AR",
            "display_name": "Ingresos — Tabla · table-main",
            "description": "Datos sintéticos"
          }
        ],
        "enabled": true,
        "default_selected": true,
        "display_order": 2
      },
      "id": "table-main",
      "kind": "table",
      "state": "succeeded",
      "code": "",
      "retained_digest": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "table": {
        "columns": [
          {
            "id": "c0",
            "name": "sale_date",
            "display_label": "Sale date",
            "type": "temporal",
            "role": "time",
            "grain": "",
            "aggregation": "",
            "format": {
              "unit": "",
              "currency": "",
              "percent": "",
              "fraction_digits": 0,
              "locale": "en-US",
              "date_pattern": "date_medium"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          },
          {
            "id": "c1",
            "name": "amount",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "revenue",
              "currency": "USD",
              "percent": "",
              "fraction_digits": 3,
              "locale": "en-US"
            },
            "provenance": {
              "version": 1,
              "source": "",
              "source_revision": 0,
              "topic": "",
              "topic_version": "",
              "semantic_id": ""
            }
          }
        ],
        "rows": [
          [
            {
              "null": false,
              "value": "2026-01-03"
            },
            {
              "null": false,
              "value": "5.500"
            }
          ]
        ],
        "totals": [
          {
            "column": "c1",
            "value": {
              "null": false,
              "value": "9007199254740998.625"
            },
            "scope": "complete_result"
          }
        ],
        "completeness": {
          "status": "complete_result",
          "reason": ""
        },
        "warnings": []
      }
    },
    "page_bounds": {
      "offset": 1,
      "limit": 1,
      "total": 2
    }
  }},
};

// This metadata catalog is explicitly synthetic. Field definitions and the final
// line preview use the same recorded sale_date/amount columns as the retained DTO.
export const browserMappingSamples={catalog:mapCatalog,columns:capturedViews.trend.output.chart.mapping.columns};

// Safe native public DTO projection for the synthetic-source integration test.
// No credentials, native control handles or customer data are included.
export function projectDatasetSamples(native){
 const keys=['dataset','list_topics','describe_topic','request','preparation','created','validation','view_root','view_output','initial_read','save_request','final_read','preview','complete'];
 const samples=Object.fromEntries(keys.filter(k=>Object.hasOwn(native,k)).map(k=>[k,JSON.parse(JSON.stringify(native[k]))]));
 const forbidden=new Set(['sql','query','attempt','remote','manifest','session','token','credential','credentials','password','dsn','connection_string']);
 function visit(value){if(!value||typeof value!=='object')return;for(const [key,item] of Object.entries(value)){if(forbidden.has(key))throw new Error('Private runtime field in public fixture: '+key);if(key==='expires_at')value[key]='2099-01-01T00:00:00Z';else if(['created_at','updated_at','observed_at','published_at'].includes(key)&&typeof item==='string')value[key]='2026-10-03T12:00:00Z';else visit(item);}}
 visit(samples);return samples;
}

// Race-tested continuous public capture SHA256 6199ddd21737495f147ef56482b4d6931062761bc965d41f721ce1263f98c449.
export const browserDatasetSamples=projectDatasetSamples({
  "dataset": {
    "compiler": "reviewed-dataset-postgres-v1",
    "topic": {
      "topic": "commerce",
      "version": "v1",
      "digest": "d306aeb1c73b5444da71a1f010db6c2dd5f567704f61cdce3d130f0e52b9f756"
    },
    "dataset": "ds:95b3449b842ed4ac7c5306c17930e519",
    "source": "topic-source",
    "context": "topic-source:v1",
    "source_revision": 1,
    "dialect": "postgres",
    "dimensions": [],
    "measures": [
      {
        "id": "revenue",
        "binding": "m_46f93a6a01617b80efa06168",
        "name": "Revenue",
        "role": "measure",
        "aggregation": "sum",
        "unit": "currency",
        "supported": true
      }
    ],
    "chart_kinds": [
      "bar",
      "column",
      "line",
      "area",
      "pie",
      "donut",
      "kpi",
      "table"
    ],
    "limitations": [
      "PostgreSQL only; zero to two direct dimensions and one reviewed measure.",
      "No filters, group policies, calendar bucketing, joins, arbitrary KPI expressions, active rules or amount-completeness policies in the initial compiler.",
      "Prepare reads actual schema. Create remains private and unvalidated; Validate is a separate explicit read."
    ],
    "supported": true
  },
  "list_topics": [
    {
      "topic": "commerce",
      "version": "v1",
      "revision": 1,
      "name": "Commerce",
      "description": "Synthetic draft",
      "digest": "d306aeb1c73b5444da71a1f010db6c2dd5f567704f61cdce3d130f0e52b9f756",
      "published_at": "2026-10-03T21:59:22.443914-03:00"
    }
  ],
  "describe_topic": {
    "state": {
      "topic": "commerce",
      "revision": 1,
      "version": "v1",
      "archived": false,
      "active": true,
      "generations": [
        {
          "context": "topic-source:v1",
          "generation": "94736e65d4c6df54950319d70a3762f240f6f447ccfd9bf7f751196bf9697236",
          "space": "d2072b06b32d7cedf8c33797de695254a38fc6f99d5ea91c515a403a13465f40",
          "facets": 9
        }
      ]
    },
    "definition": {
      "schema_version": 1,
      "topic": "commerce",
      "version": "v1",
      "name": "Commerce",
      "description": "Synthetic draft",
      "datasets": [
        {
          "id": "ds:9191829286eb6e0094d70fda79214271",
          "name": "Items",
          "source": {
            "source": "topic-source",
            "context": "topic-source:v1",
            "dataset": "ds:9191829286eb6e0094d70fda79214271",
            "source_revision": 1
          },
          "columns": [
            {
              "sensitivity": "non_sensitive",
              "id": "quantity",
              "source_name": "quantity",
              "name": "quantity",
              "native_type": "integer",
              "category": "numeric",
              "nullable": true
            },
            {
              "sensitivity": "non_sensitive",
              "id": "sale_id",
              "source_name": "sale_id",
              "name": "sale_id",
              "native_type": "integer",
              "category": "numeric",
              "nullable": true
            }
          ]
        },
        {
          "id": "ds:95b3449b842ed4ac7c5306c17930e519",
          "name": "Sales",
          "source": {
            "source": "topic-source",
            "context": "topic-source:v1",
            "dataset": "ds:95b3449b842ed4ac7c5306c17930e519",
            "source_revision": 1
          },
          "columns": [
            {
              "sensitivity": "non_sensitive",
              "id": "amount",
              "source_name": "amount",
              "name": "amount",
              "native_type": "numeric(30,3)",
              "category": "numeric",
              "nullable": true
            },
            {
              "sensitivity": "non_sensitive",
              "id": "id",
              "source_name": "id",
              "name": "id",
              "native_type": "integer",
              "category": "numeric",
              "nullable": false
            }
          ]
        }
      ],
      "measures": [
        {
          "id": "revenue",
          "name": "Revenue",
          "description": "Total amount",
          "field": {
            "kind": "column",
            "dataset": "ds:95b3449b842ed4ac7c5306c17930e519",
            "id": "amount"
          },
          "aggregation": "sum",
          "unit": "currency"
        }
      ],
      "dimensions": null,
      "kpis": null,
      "joins": [
        {
          "id": "sales-items",
          "name": "Sales to items",
          "left": {
            "kind": "column",
            "dataset": "ds:95b3449b842ed4ac7c5306c17930e519",
            "id": "id"
          },
          "right": {
            "kind": "column",
            "dataset": "ds:9191829286eb6e0094d70fda79214271",
            "id": "sale_id"
          },
          "type": "inner",
          "cardinality": "one_to_one",
          "evidence": {}
        }
      ],
      "canonical_entities": null
    },
    "digest": "d306aeb1c73b5444da71a1f010db6c2dd5f567704f61cdce3d130f0e52b9f756",
    "published_at": "2026-10-03T21:59:22.443914-03:00"
  },
  "request": {
    "new_block": "dataset-private-chart",
    "operation": "dataset-prepare-one",
    "intent": {
      "topic": {
        "topic": "commerce",
        "version": "v1",
        "digest": "d306aeb1c73b5444da71a1f010db6c2dd5f567704f61cdce3d130f0e52b9f756"
      },
      "dataset": "ds:95b3449b842ed4ac7c5306c17930e519",
      "dimensions": [],
      "measure": "revenue",
      "mapping": {
        "kind": "kpi",
        "bindings": {
          "value": "m_46f93a6a01617b80efa06168"
        },
        "order": [],
        "options": {
          "title": "Prepared revenue",
          "legend": {
            "visible": true,
            "position": "bottom"
          },
          "label_max_runes": 80
        }
      }
    },
    "metadata": [
      {
        "locale": "en-US",
        "title": "Prepared revenue",
        "question": "Prepared revenue",
        "aliases": [],
        "description": ""
      }
    ]
  },
  "preparation": {
    "preparation": "261d87a6651f78c87c8b2a84f244e98f",
    "new_block": "dataset-private-chart",
    "operation": "dataset-prepare-one",
    "digest": "99f7a0a4b802fc8c865273b884cc695c8e0aaafdf6a4faa22305479fd7fb2b6f",
    "status": "prepared",
    "schema": [
      {
        "name": "m_46f93a6a01617b80efa06168",
        "type": "decimal",
        "encoding": "string",
        "native_type": "numeric"
      }
    ],
    "mapping": {
      "version": 1,
      "kind": "kpi",
      "columns": [
        {
          "id": "m_46f93a6a01617b80efa06168",
          "name": "m_46f93a6a01617b80efa06168",
          "display_label": "Revenue",
          "type": "decimal",
          "role": "measure",
          "grain": "",
          "aggregation": "sum",
          "format": {
            "unit": "currency",
            "currency": "",
            "percent": "",
            "fraction_digits": 0
          },
          "provenance": {
            "version": 1,
            "source": "topic-source",
            "source_revision": 1,
            "topic": "commerce",
            "topic_version": "v1",
            "semantic_id": "revenue"
          }
        }
      ],
      "bindings": {
        "value": "m_46f93a6a01617b80efa06168"
      },
      "order": [],
      "options": {
        "title": "Prepared revenue",
        "legend": {
          "visible": true,
          "position": "bottom"
        },
        "label_max_runes": 80
      }
    },
    "expires_at": "2026-10-04T01:14:22.693378921Z",
    "validation": "native_validation_required",
    "execution_status": "succeeded",
    "remote_state": "stopped"
  },
  "created": {
    "block": {
      "schema_version": 1,
      "result_policy": [
        {
          "field": "m_46f93a6a01617b80efa06168",
          "status": "unknown",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "fe924593cb68277b37dd206228f1aa9cf56497b06bd69a043345a5b266a6a5a2"
        }
      ],
      "state": {
        "id": "dataset-private-chart",
        "topic": "commerce",
        "version": 1,
        "draft_revision": 1,
        "published_revision": 0,
        "draft_state": "draft",
        "archived": false,
        "created_at": "2026-10-03T21:59:22.932112-03:00",
        "updated_at": "2026-10-03T21:59:22.932112-03:00"
      },
      "revision": 1,
      "revision_id": "3f61159978a130d0a442a4d2132bf02a",
      "digest": "6d927d49f94749fabbd8a3fcaa776a5985f0acd3b5a7de33d8432b388edb65c6",
      "execution_digest": "e918a26d2e97767bb56ceaf93b9fe403f151a9502b8a5ff8db0fb15112244f10",
      "metadata": [
        {
          "locale": "en-US",
          "title": "Prepared revenue",
          "question": "Prepared revenue",
          "aliases": [],
          "description": ""
        }
      ],
      "source": "topic-source",
      "context": "topic-source:v1",
      "topics": [
        {
          "topic": "commerce",
          "version": "v1",
          "digest": "d306aeb1c73b5444da71a1f010db6c2dd5f567704f61cdce3d130f0e52b9f756"
        }
      ],
      "parameters": [],
      "expected_schema": [
        {
          "name": "m_46f93a6a01617b80efa06168",
          "type": "decimal",
          "encoding": "string",
          "native_type": "numeric"
        }
      ],
      "outputs": [
        {
          "id": "chart",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Prepared revenue — Indicator · chart",
                "description": ""
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "mapping": {
            "version": 1,
            "kind": "kpi",
            "columns": [
              {
                "id": "m_46f93a6a01617b80efa06168",
                "name": "m_46f93a6a01617b80efa06168",
                "display_label": "Revenue",
                "type": "decimal",
                "role": "measure",
                "grain": "",
                "aggregation": "sum",
                "format": {
                  "unit": "currency",
                  "currency": "",
                  "percent": "",
                  "fraction_digits": 0
                },
                "provenance": {
                  "version": 1,
                  "source": "topic-source",
                  "source_revision": 1,
                  "topic": "commerce",
                  "topic_version": "v1",
                  "semantic_id": "revenue"
                }
              }
            ],
            "bindings": {
              "value": "m_46f93a6a01617b80efa06168"
            },
            "order": [],
            "options": {
              "title": "Prepared revenue",
              "legend": {
                "visible": true,
                "position": "bottom"
              },
              "label_max_runes": 80
            }
          },
          "editable": true
        }
      ],
      "actor": "operator",
      "created_at": "2026-10-04T00:59:22.773307654Z",
      "private": true,
      "trust": {
        "publication": "draft",
        "certification": "none",
        "health": {
          "status": "unknown",
          "reason": "not_checked"
        }
      }
    },
    "output_columns": [
      {
        "output": "chart",
        "columns": [
          {
            "id": "m_46f93a6a01617b80efa06168",
            "name": "m_46f93a6a01617b80efa06168",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "currency",
              "currency": "",
              "percent": "",
              "fraction_digits": 0
            },
            "provenance": {
              "version": 1,
              "source": "topic-source",
              "source_revision": 1,
              "topic": "commerce",
              "topic_version": "v1",
              "semantic_id": "revenue"
            }
          }
        ]
      }
    ],
    "data_validation": "not_performed"
  },
  "validation": {
    "state": {
      "id": "dataset-private-chart",
      "topic": "commerce",
      "version": 2,
      "draft_revision": 1,
      "published_revision": 0,
      "draft_state": "validated",
      "archived": false,
      "created_at": "2026-10-03T21:59:22.932112-03:00",
      "updated_at": "2026-10-03T21:59:23.269642-03:00"
    },
    "evidence": {
      "id": "787784dc0afcd992f690c27e6ea3b05f",
      "revision": 1,
      "definition_digest": "6d927d49f94749fabbd8a3fcaa776a5985f0acd3b5a7de33d8432b388edb65c6",
      "execution_digest": "e918a26d2e97767bb56ceaf93b9fe403f151a9502b8a5ff8db0fb15112244f10",
      "schema_digest": "f408819d52279e8d85fa4a42073d0eee36676d36b4647fadd7bbecb01dcd2d9b",
      "schema": [
        {
          "name": "m_46f93a6a01617b80efa06168",
          "type": "decimal",
          "encoding": "string",
          "native_type": "numeric"
        }
      ],
      "created_at": "2026-10-04T00:59:23.210123204Z",
      "expires_at": "2026-10-05T00:59:23.210123204Z"
    }
  },
  "view_root": {
    "accepted_selection": {
      "definition_version": 1,
      "version": 2,
      "mode": "explicit",
      "requested": [
        "chart"
      ],
      "selected": [
        "chart"
      ],
      "choices": [
        {
          "id": "chart",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Prepared revenue — Indicator · chart",
                "description": ""
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "selected": true,
          "state": "selected"
        }
      ]
    },
    "query_limits": {
      "max_rows": 1000,
      "max_bytes": 1048576,
      "timeout_ms": 60000,
      "query_attempts": 3
    },
    "version": "reporting-view-v1",
    "summary": {
      "kind": "report",
      "run": "ee459a83526ce05123da4bafe458aa60",
      "target": {
        "kind": "report",
        "id": "dataset-private-report",
        "revision": 2
      },
      "state": "completed",
      "code": "",
      "private": true,
      "created_at": "2026-10-03T21:59:23.31805-03:00",
      "expires_at": "2026-10-04T21:59:23.31805-03:00"
    },
    "selection": {
      "kind": "report",
      "run": "ee459a83526ce05123da4bafe458aa60",
      "page": "analysis",
      "widget": "widget-c0ffee02",
      "output": "chart",
      "offset": 0,
      "limit": 100
    },
    "locale": "en-US",
    "timezone": "UTC",
    "outputs": [
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Prepared revenue — Indicator · chart",
            "description": ""
          }
        ],
        "description": "",
        "locale": "en-US",
        "display_order": 0,
        "enabled": true,
        "default_selected": true,
        "selected": true,
        "state": "selected",
        "id": "chart",
        "kind": "kpi",
        "title": "Prepared revenue — Indicator · chart"
      }
    ],
    "pages": [
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "analysis",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Analysis",
        "widgets": [
          {
            "selection": {
              "definition_version": 1,
              "version": 2,
              "mode": "explicit",
              "requested": [
                "chart"
              ],
              "selected": [
                "chart"
              ],
              "choices": [
                {
                  "id": "chart",
                  "kind": "kpi",
                  "intent": {
                    "metadata": [
                      {
                        "locale": "en-US",
                        "display_name": "Prepared revenue — Indicator · chart",
                        "description": ""
                      }
                    ],
                    "enabled": true,
                    "default_selected": true,
                    "display_order": 0
                  },
                  "selected": true,
                  "state": "selected"
                }
              ]
            },
            "query_limits": {
              "max_rows": 1000,
              "max_bytes": 1048576,
              "timeout_ms": 60000,
              "query_attempts": 3
            },
            "id": "widget-c0ffee02",
            "kind": "block",
            "state": "completed",
            "grid": {
              "column": 0,
              "row": 0,
              "width": 4,
              "height": 3
            },
            "presentation": {
              "title": "Prepared revenue"
            },
            "outputs": [
              "chart"
            ],
            "parameters": [],
            "trust": {
              "publication": "draft",
              "certification": "none",
              "health": {
                "status": "healthy",
                "observed_at": "2026-10-04T00:59:23.210123204Z",
                "dependency_digest": "2a0291b4d27cfdb36d1f11722ffce31b71ca6429a5b5e56d96e6fa94540e4797",
                "reason": "validated_observation"
              }
            },
            "observed_at": "2026-10-03T21:59:23.836135-03:00"
          }
        ]
      },
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "empty",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Empty",
        "widgets": []
      }
    ],
    "filters": [],
    "trust": {
      "publication": "draft",
      "certification": "none",
      "health": {
        "status": "healthy",
        "observed_at": "2026-10-04T00:59:23.210123204Z",
        "dependency_digest": "2a0291b4d27cfdb36d1f11722ffce31b71ca6429a5b5e56d96e6fa94540e4797",
        "reason": "validated_observation"
      }
    },
    "observed_at": "2026-10-03T21:59:23.836135-03:00",
    "mixed_freshness": false,
    "redacted": false,
    "output": {
      "result_policy": [
        {
          "field": "m_46f93a6a01617b80efa06168",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "fe35958b8f1a334a933c95a3cb7bc816b4f346313143da469a73e12dc5803ba1"
        }
      ],
      "intent": {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Prepared revenue — Indicator · chart",
            "description": ""
          }
        ],
        "enabled": true,
        "default_selected": true,
        "display_order": 0
      },
      "id": "chart",
      "kind": "kpi",
      "state": "succeeded",
      "code": "",
      "retained_digest": "2aa52aeb03cc9b5ced07d98fb71e6352a22ff2c3eaedc7b2cb72b09dcaf79f11",
      "chart": {
        "version": 1,
        "kind": "kpi",
        "mapping": {
          "version": 1,
          "kind": "kpi",
          "columns": [
            {
              "id": "m_46f93a6a01617b80efa06168",
              "name": "m_46f93a6a01617b80efa06168",
              "display_label": "Revenue",
              "type": "decimal",
              "role": "measure",
              "grain": "",
              "aggregation": "sum",
              "format": {
                "unit": "currency",
                "currency": "",
                "percent": "",
                "fraction_digits": 0
              },
              "provenance": {
                "version": 1,
                "source": "topic-source",
                "source_revision": 1,
                "topic": "commerce",
                "topic_version": "v1",
                "semantic_id": "revenue"
              }
            }
          ],
          "bindings": {
            "value": "m_46f93a6a01617b80efa06168"
          },
          "order": [],
          "options": {
            "title": "Prepared revenue",
            "legend": {
              "visible": true,
              "position": "bottom"
            },
            "label_max_runes": 80
          }
        },
        "columns": [
          {
            "id": "m_46f93a6a01617b80efa06168",
            "name": "m_46f93a6a01617b80efa06168",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "currency",
              "currency": "",
              "percent": "",
              "fraction_digits": 0
            },
            "provenance": {
              "version": 1,
              "source": "topic-source",
              "source_revision": 1,
              "topic": "commerce",
              "topic_version": "v1",
              "semantic_id": "revenue"
            }
          }
        ],
        "rows": [],
        "points": [
          {
            "row": 0,
            "category": {
              "null": true,
              "value": ""
            },
            "series": {
              "null": true,
              "value": ""
            },
            "parent": {
              "null": true,
              "value": ""
            },
            "x": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "y": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "value": {
              "null": false,
              "exact": "9007199254740998.625",
              "coordinate": 9007199254740998,
              "approximate": true
            }
          }
        ],
        "totals": [
          {
            "column": "m_46f93a6a01617b80efa06168",
            "value": {
              "null": false,
              "value": "9007199254740998.625"
            },
            "scope": "complete_result"
          }
        ],
        "state": "ready",
        "input_rows": 1,
        "omitted_rows": 0,
        "completeness": {
          "status": "complete_result",
          "reason": ""
        },
        "warnings": [
          "geometry_approximate_labels_exact"
        ]
      }
    },
    "page_bounds": {
      "offset": 0,
      "limit": 100,
      "total": 1
    }
  },
  "view_output": {
    "accepted_selection": {
      "definition_version": 1,
      "version": 2,
      "mode": "explicit",
      "requested": [
        "chart"
      ],
      "selected": [
        "chart"
      ],
      "choices": [
        {
          "id": "chart",
          "kind": "kpi",
          "intent": {
            "metadata": [
              {
                "locale": "en-US",
                "display_name": "Prepared revenue — Indicator · chart",
                "description": ""
              }
            ],
            "enabled": true,
            "default_selected": true,
            "display_order": 0
          },
          "selected": true,
          "state": "selected"
        }
      ]
    },
    "query_limits": {
      "max_rows": 1000,
      "max_bytes": 1048576,
      "timeout_ms": 60000,
      "query_attempts": 3
    },
    "version": "reporting-view-v1",
    "summary": {
      "kind": "report",
      "run": "ee459a83526ce05123da4bafe458aa60",
      "target": {
        "kind": "report",
        "id": "dataset-private-report",
        "revision": 2
      },
      "state": "completed",
      "code": "",
      "private": true,
      "created_at": "2026-10-03T21:59:23.31805-03:00",
      "expires_at": "2026-10-04T21:59:23.31805-03:00"
    },
    "selection": {
      "kind": "report",
      "run": "ee459a83526ce05123da4bafe458aa60",
      "page": "analysis",
      "widget": "widget-c0ffee02",
      "output": "chart",
      "offset": 0,
      "limit": 100
    },
    "locale": "en-US",
    "timezone": "UTC",
    "outputs": [
      {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Prepared revenue — Indicator · chart",
            "description": ""
          }
        ],
        "description": "",
        "locale": "en-US",
        "display_order": 0,
        "enabled": true,
        "default_selected": true,
        "selected": true,
        "state": "selected",
        "id": "chart",
        "kind": "kpi",
        "title": "Prepared revenue — Indicator · chart"
      }
    ],
    "pages": [
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "analysis",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Analysis",
        "widgets": [
          {
            "selection": {
              "definition_version": 1,
              "version": 2,
              "mode": "explicit",
              "requested": [
                "chart"
              ],
              "selected": [
                "chart"
              ],
              "choices": [
                {
                  "id": "chart",
                  "kind": "kpi",
                  "intent": {
                    "metadata": [
                      {
                        "locale": "en-US",
                        "display_name": "Prepared revenue — Indicator · chart",
                        "description": ""
                      }
                    ],
                    "enabled": true,
                    "default_selected": true,
                    "display_order": 0
                  },
                  "selected": true,
                  "state": "selected"
                }
              ]
            },
            "query_limits": {
              "max_rows": 1000,
              "max_bytes": 1048576,
              "timeout_ms": 60000,
              "query_attempts": 3
            },
            "id": "widget-c0ffee02",
            "kind": "block",
            "state": "completed",
            "grid": {
              "column": 0,
              "row": 0,
              "width": 4,
              "height": 3
            },
            "presentation": {
              "title": "Prepared revenue"
            },
            "outputs": [
              "chart"
            ],
            "parameters": [],
            "trust": {
              "publication": "draft",
              "certification": "none",
              "health": {
                "status": "healthy",
                "observed_at": "2026-10-04T00:59:23.210123204Z",
                "dependency_digest": "2a0291b4d27cfdb36d1f11722ffce31b71ca6429a5b5e56d96e6fa94540e4797",
                "reason": "validated_observation"
              }
            },
            "observed_at": "2026-10-03T21:59:23.836135-03:00"
          }
        ]
      },
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "empty",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Empty",
        "widgets": []
      }
    ],
    "filters": [],
    "trust": {
      "publication": "draft",
      "certification": "none",
      "health": {
        "status": "healthy",
        "observed_at": "2026-10-04T00:59:23.210123204Z",
        "dependency_digest": "2a0291b4d27cfdb36d1f11722ffce31b71ca6429a5b5e56d96e6fa94540e4797",
        "reason": "validated_observation"
      }
    },
    "observed_at": "2026-10-03T21:59:23.836135-03:00",
    "mixed_freshness": false,
    "redacted": false,
    "output": {
      "result_policy": [
        {
          "field": "m_46f93a6a01617b80efa06168",
          "status": "allowed",
          "basis": "reviewed_query_dependencies",
          "provenance_digest": "fe35958b8f1a334a933c95a3cb7bc816b4f346313143da469a73e12dc5803ba1"
        }
      ],
      "intent": {
        "metadata": [
          {
            "locale": "en-US",
            "display_name": "Prepared revenue — Indicator · chart",
            "description": ""
          }
        ],
        "enabled": true,
        "default_selected": true,
        "display_order": 0
      },
      "id": "chart",
      "kind": "kpi",
      "state": "succeeded",
      "code": "",
      "retained_digest": "2aa52aeb03cc9b5ced07d98fb71e6352a22ff2c3eaedc7b2cb72b09dcaf79f11",
      "chart": {
        "version": 1,
        "kind": "kpi",
        "mapping": {
          "version": 1,
          "kind": "kpi",
          "columns": [
            {
              "id": "m_46f93a6a01617b80efa06168",
              "name": "m_46f93a6a01617b80efa06168",
              "display_label": "Revenue",
              "type": "decimal",
              "role": "measure",
              "grain": "",
              "aggregation": "sum",
              "format": {
                "unit": "currency",
                "currency": "",
                "percent": "",
                "fraction_digits": 0
              },
              "provenance": {
                "version": 1,
                "source": "topic-source",
                "source_revision": 1,
                "topic": "commerce",
                "topic_version": "v1",
                "semantic_id": "revenue"
              }
            }
          ],
          "bindings": {
            "value": "m_46f93a6a01617b80efa06168"
          },
          "order": [],
          "options": {
            "title": "Prepared revenue",
            "legend": {
              "visible": true,
              "position": "bottom"
            },
            "label_max_runes": 80
          }
        },
        "columns": [
          {
            "id": "m_46f93a6a01617b80efa06168",
            "name": "m_46f93a6a01617b80efa06168",
            "display_label": "Revenue",
            "type": "decimal",
            "role": "measure",
            "grain": "",
            "aggregation": "sum",
            "format": {
              "unit": "currency",
              "currency": "",
              "percent": "",
              "fraction_digits": 0
            },
            "provenance": {
              "version": 1,
              "source": "topic-source",
              "source_revision": 1,
              "topic": "commerce",
              "topic_version": "v1",
              "semantic_id": "revenue"
            }
          }
        ],
        "rows": [],
        "points": [
          {
            "row": 0,
            "category": {
              "null": true,
              "value": ""
            },
            "series": {
              "null": true,
              "value": ""
            },
            "parent": {
              "null": true,
              "value": ""
            },
            "x": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "y": {
              "null": true,
              "exact": "",
              "coordinate": null,
              "approximate": false
            },
            "value": {
              "null": false,
              "exact": "9007199254740998.625",
              "coordinate": 9007199254740998,
              "approximate": true
            }
          }
        ],
        "totals": [
          {
            "column": "m_46f93a6a01617b80efa06168",
            "value": {
              "null": false,
              "value": "9007199254740998.625"
            },
            "scope": "complete_result"
          }
        ],
        "state": "ready",
        "input_rows": 1,
        "omitted_rows": 0,
        "completeness": {
          "status": "complete_result",
          "reason": ""
        },
        "warnings": [
          "geometry_approximate_labels_exact"
        ]
      }
    },
    "page_bounds": {
      "offset": 0,
      "limit": 100,
      "total": 1
    }
  },
  "initial_read": {
    "state": {
      "kind": "report",
      "id": "dataset-private-report",
      "version": 1,
      "latest_revision": 1,
      "draft_revision": 1,
      "review_revision": 0,
      "published_revision": 0,
      "archived": false,
      "created_at": "2026-10-03T21:59:22.657969-03:00",
      "updated_at": "2026-10-03T21:59:22.657969-03:00"
    },
    "revision": 1,
    "digest": "3ecba0fa42d5c8c547c5fa93938806bd10e9eec4341367ce32ab3e46972a4352",
    "private": true,
    "definition": {
      "schema_version": 3,
      "metadata": [
        {
          "locale": "en-US",
          "title": "Dataset chart report"
        },
        {
          "locale": "es-AR",
          "title": "Informe sintético"
        }
      ],
      "locale": "en-US",
      "timezone": "UTC",
      "report_pages": [
        {
          "id": "analysis",
          "title": "Analysis",
          "widgets": []
        },
        {
          "id": "empty",
          "title": "Empty",
          "widgets": []
        }
      ],
      "partial_failure": "fail_closed"
    }
  },
  "save_request": {
    "report": "dataset-private-report",
    "expected_version": 1,
    "revision": 1,
    "definition": {
      "schema_version": 3,
      "metadata": [
        {
          "locale": "en-US",
          "title": "Dataset chart report"
        },
        {
          "locale": "es-AR",
          "title": "Informe sintético"
        }
      ],
      "locale": "en-US",
      "timezone": "UTC",
      "report_pages": [
        {
          "id": "analysis",
          "title": "Analysis",
          "widgets": [
            {
              "id": "widget-c0ffee02",
              "kind": "block",
              "grid": {
                "column": 0,
                "row": 0,
                "width": 4,
                "height": 3
              },
              "presentation": {
                "title": "Prepared revenue"
              },
              "block": {
                "digest": "6d927d49f94749fabbd8a3fcaa776a5985f0acd3b5a7de33d8432b388edb65c6",
                "block": "dataset-private-chart",
                "revision": 1,
                "outputs": [
                  "chart"
                ],
                "policy": "private_preview",
                "narrative": false
              }
            }
          ]
        },
        {
          "id": "empty",
          "title": "Empty",
          "widgets": []
        }
      ],
      "partial_failure": "fail_closed"
    }
  },
  "final_read": {
    "state": {
      "kind": "report",
      "id": "dataset-private-report",
      "version": 2,
      "latest_revision": 2,
      "draft_revision": 2,
      "review_revision": 0,
      "published_revision": 0,
      "archived": false,
      "created_at": "2026-10-03T21:59:22.657969-03:00",
      "updated_at": "2026-10-03T21:59:23.063437-03:00"
    },
    "revision": 2,
    "digest": "c1e1f7051d83103d0ef9af830be50e59dba540ae24a0b169558711deb799b3cb",
    "private": true,
    "definition": {
      "schema_version": 3,
      "metadata": [
        {
          "locale": "en-US",
          "title": "Dataset chart report"
        },
        {
          "locale": "es-AR",
          "title": "Informe sintético"
        }
      ],
      "locale": "en-US",
      "timezone": "UTC",
      "report_pages": [
        {
          "id": "analysis",
          "title": "Analysis",
          "widgets": [
            {
              "id": "widget-c0ffee02",
              "kind": "block",
              "grid": {
                "column": 0,
                "row": 0,
                "width": 4,
                "height": 3
              },
              "presentation": {
                "title": "Prepared revenue"
              },
              "block": {
                "digest": "6d927d49f94749fabbd8a3fcaa776a5985f0acd3b5a7de33d8432b388edb65c6",
                "block": "dataset-private-chart",
                "revision": 1,
                "outputs": [
                  "chart"
                ],
                "policy": "private_preview",
                "narrative": false
              }
            }
          ]
        },
        {
          "id": "empty",
          "title": "Empty",
          "widgets": []
        }
      ],
      "partial_failure": "fail_closed"
    }
  },
  "preview": {
    "id": "ee459a83526ce05123da4bafe458aa60",
    "kind": "report",
    "document": "dataset-private-report",
    "revision": 2,
    "manifest_digest": "cccc11974a8cbafac3a16bfce71bb76706eab05771e5d75dec0e30d460e43a16",
    "state": "sealed",
    "private": true,
    "complete": false,
    "mixed_freshness": false,
    "redacted": false,
    "created_at": "2026-10-04T00:59:23.31805Z",
    "expires_at": "2026-10-05T00:59:23.31805Z",
    "pages": [
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "analysis",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Analysis",
        "widgets": [
          {
            "selection": {
              "definition_version": 1,
              "version": 2,
              "mode": "explicit",
              "requested": [
                "chart"
              ],
              "selected": [
                "chart"
              ],
              "choices": [
                {
                  "id": "chart",
                  "kind": "kpi",
                  "intent": {
                    "metadata": [
                      {
                        "locale": "en-US",
                        "display_name": "Prepared revenue — Indicator · chart",
                        "description": ""
                      }
                    ],
                    "enabled": true,
                    "default_selected": true,
                    "display_order": 0
                  },
                  "selected": true,
                  "state": "selected"
                }
              ]
            },
            "query_limits": {
              "max_rows": 1000,
              "max_bytes": 1048576,
              "timeout_ms": 60000,
              "query_attempts": 3
            },
            "id": "widget-c0ffee02",
            "kind": "block",
            "state": "pending",
            "grid": {
              "column": 0,
              "row": 0,
              "width": 4,
              "height": 3
            },
            "presentation": {
              "title": "Prepared revenue"
            },
            "outputs": [
              "chart"
            ],
            "parameters": [],
            "trust": {
              "publication": "draft",
              "certification": "none",
              "health": {
                "status": "healthy",
                "observed_at": "2026-10-04T00:59:23.210123204Z",
                "dependency_digest": "2a0291b4d27cfdb36d1f11722ffce31b71ca6429a5b5e56d96e6fa94540e4797",
                "reason": "validated_observation"
              }
            }
          }
        ]
      },
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "empty",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Empty",
        "widgets": []
      }
    ],
    "query_groups": 1,
    "retained_bytes": 5585
  },
  "complete": {
    "id": "ee459a83526ce05123da4bafe458aa60",
    "kind": "report",
    "document": "dataset-private-report",
    "revision": 2,
    "manifest_digest": "cccc11974a8cbafac3a16bfce71bb76706eab05771e5d75dec0e30d460e43a16",
    "state": "completed",
    "private": true,
    "complete": true,
    "mixed_freshness": false,
    "redacted": false,
    "created_at": "2026-10-04T00:59:23.31805Z",
    "expires_at": "2026-10-05T00:59:23.31805Z",
    "finished_at": "2026-10-03T21:59:24.103665-03:00",
    "pages": [
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "analysis",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Analysis",
        "widgets": [
          {
            "selection": {
              "definition_version": 1,
              "version": 2,
              "mode": "explicit",
              "requested": [
                "chart"
              ],
              "selected": [
                "chart"
              ],
              "choices": [
                {
                  "id": "chart",
                  "kind": "kpi",
                  "intent": {
                    "metadata": [
                      {
                        "locale": "en-US",
                        "display_name": "Prepared revenue — Indicator · chart",
                        "description": ""
                      }
                    ],
                    "enabled": true,
                    "default_selected": true,
                    "display_order": 0
                  },
                  "selected": true,
                  "state": "selected"
                }
              ]
            },
            "query_limits": {
              "max_rows": 1000,
              "max_bytes": 1048576,
              "timeout_ms": 60000,
              "query_attempts": 3
            },
            "id": "widget-c0ffee02",
            "kind": "block",
            "state": "completed",
            "grid": {
              "column": 0,
              "row": 0,
              "width": 4,
              "height": 3
            },
            "presentation": {
              "title": "Prepared revenue"
            },
            "outputs": [
              "chart"
            ],
            "parameters": [],
            "trust": {
              "publication": "draft",
              "certification": "none",
              "health": {
                "status": "healthy",
                "observed_at": "2026-10-04T00:59:23.210123204Z",
                "dependency_digest": "2a0291b4d27cfdb36d1f11722ffce31b71ca6429a5b5e56d96e6fa94540e4797",
                "reason": "validated_observation"
              }
            },
            "observed_at": "2026-10-03T21:59:23.836135-03:00"
          }
        ]
      },
      {
        "locale": "en-US",
        "timezone": "UTC",
        "id": "empty",
        "report": "dataset-private-report",
        "revision": 2,
        "title": "Empty",
        "widgets": []
      }
    ],
    "query_groups": 1,
    "retained_bytes": 10978
  }
});

// Stringified into the isolated test host. All state is disposable and belongs to
// this fixture; the compiled application receives only normal tool DTOs.
export function initializeSyntheticHost(samples, embedded, mappingSamples, datasetSamples) {
  const frame=document.getElementById('app');
  const clone=value=>JSON.parse(JSON.stringify(value));
  const wrap=value=>({structuredContent:{result:clone(value)}});
  const failure=code=>({isError:true,structuredContent:{error:{code,outcome:'not_started'}}});
  Object.assign(window,{calls:[],lastResizeAt:0,resizeCount:0,resizeMessages:[],saveDelay:0,viewDelay:0,rejectSave:false,denyPrivate:false,consumerOnly:false,publishedPagesCatalog:false,compactCatalog:false,denial:'',privateRuns:new Set(),waiting:[]});
  window.state={id:'report-a',kind:'report',version:4,draft_revision:3,published_revision:2};
  window.definition={schema_version:2,metadata:[{locale:'en-US',title:'Weekly operations'}],locale:'en-US',timezone:'UTC',partial_failure:'fail_closed',widgets:[{id:'intro',kind:'text',grid:{column:0,row:0,width:12,height:1},presentation:{},text:{format:'plain',text:'Synthetic weekly overview'}}],filters:[]};
  const target={kind:'report',id:'report-a',revision:2};
  const filter={page:'main',label:'Maximum rows',parameter:{name:'maximum',type:'integer',required:false,default:{literal:'10'},min:'1',max:'100'}};
  const outputTitles={'kpi-main':'Synthetic revenue KPI','trend-main':'Synthetic revenue trend','table-main':'Synthetic revenue detail'};
  window.publishedDefinition=clone(definition);
  publishedDefinition.widgets=samples.heading.pages[0].widgets.map(widget=>widget.kind==='text'?clone(definition.widgets[0]):{
    id:widget.id,kind:'block',grid:clone(widget.grid),presentation:{title:outputTitles[widget.outputs[0]]},
    block:{block:'approved-block',revision:7,outputs:clone(widget.outputs),policy:'published',narrative:false}
  });
  // This visual host keeps the real recorded output shapes, with a separate
  // deterministic presentation revision sized for the fixed logical grid.
  const visualGrid=[{column:0,row:0,width:12,height:1},{column:0,row:1,width:4,height:5},{column:4,row:1,width:8,height:5},{column:0,row:6,width:12,height:4}];
  publishedDefinition.widgets.forEach((widget,index)=>widget.grid=clone(visualGrid[index]));
  window.compactDefinition=clone(publishedDefinition);compactDefinition.widgets.forEach(widget=>widget.grid.height=1);
  const runDefinitions=new Map([['retained-one',{definition:clone(publishedDefinition),revision:2,private:false}],['retained-compact',{definition:clone(compactDefinition),revision:1,private:false}]]);
  const reportPages=d=>d.schema_version===3?d.report_pages:[{id:'main',title:'Synthetic report canvas',widgets:d.widgets,filters:d.filters,defaults:d.defaults}];
  const sampleFor=output=>output==='kpi-main'?samples.kpi:output==='trend-main'?samples.trend:samples.table;
  const disclosure=()=>({label:'Synthetic known revenue',evidence:'reviewed_definition',definition_digest:'b'.repeat(64),declaration:'synthetic-known-revenue',value_field:'amount',unknown_count_field:'unknown_amount_count',query_outcome:'succeeded',rows_scope:'returned_query_rows',role:'amount',unit:'USD',result:{policy:'reviewed-amount-completeness-v1',scope:'returned_query_rows',metric:'synthetic:known_revenue',value_column:1,unknown_count_metric:'synthetic:unknown_amount_count',unknown_count_column:2,status:'incomplete',rows:[{row:0,status:'incomplete',unknown_count:'2'}]}});
  window.makeView=request=>{
    const run=runDefinitions.get(request.run);if(!run)return null;
    const d=run.definition,pages=reportPages(d),page=request.page||pages[0].id,p=pages.find(p=>p.id===page);if(!p)return null;
    const widget=p.widgets.find(w=>w.id===(request.widget||p.widgets[0]?.id)),empty=!p.widgets.length&&!request.widget&&!request.output;
    if(!widget&&!empty)return null;
    const output=request.output||(widget?.block?.outputs[0]||'');
    if(widget&&(widget.kind==='text'?output!=='':!widget.block.outputs.includes(output)))return null;
    const source=!widget||widget.kind==='text'?samples.heading:output==='table-main'&&request.offset===1?samples.tableNext:sampleFor(output);
    const view=clone(source),limit=request.limit||100;
    if(window.pageVisualValues){
      // Deliberate synthetic visual-data projection. Original precision samples
      // remain untouched and are fully tested before this separate scenario.
      const numbers={'9007199254740993.125':'12450.125','5.500':'13200.500','9007199254740998.625':'25650.625'};
      const project=value=>{if(!value||typeof value!=='object')return;for(const key of Object.keys(value)){if(typeof value[key]==='string'&&numbers[value[key]])value[key]=numbers[value[key]];else project(value[key]);}if(typeof value.exact==='string'&&value.exact){value.coordinate=Number(value.exact);value.approximate=false;}};
      project(view.output);if(view.output?.chart)view.output.chart.warnings=[];
    }
    view.summary={...view.summary,kind:'report',run:request.run,target:{...target,id:run.report||target.id,revision:run.revision},private:run.private,state:'completed',created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'};
    view.selection={kind:'report',run:request.run,page,widget:widget?.id||'',output,offset:request.offset||0,limit};
    view.locale=p.locale||d.locale;view.timezone=p.timezone||d.timezone;view.filters=d.schema_version===3?pages.flatMap(p=>(p.filters||[]).map(f=>({...clone(f),page:p.id}))):[filter];
    view.pages=pages.map(p=>({id:p.id,title:p.title,report:run.report||target.id,revision:run.revision,widgets:p.widgets.map(w=>({id:w.id,kind:w.kind,state:'completed',grid:clone(w.grid),presentation:clone(w.presentation),outputs:w.block?clone(w.block.outputs):[]}))}));
    view.outputs=widget?.block?widget.block.outputs.map(id=>({id,kind:sampleFor(id).output.kind,title:outputTitles[id],enabled:true,selected:true})):[];
    if(widget?.kind==='text')view.text=clone(widget.text);
    if(empty){delete view.text;delete view.output;view.page_bounds={offset:0,limit,total:0};}
    if(widget?.block?.policy==='private_preview'){
      const chart=blockViews.get(widget.block.block+'@'+widget.block.revision),mapping=chart?.block.outputs.find(o=>o.id===output)?.mapping;
      // Do not pretend the old retained sample implements a newly selected kind.
      // The browser journey returns to the recorded line shape before preview.
      if(!chart?.block.validation||chart.block.digest!==widget.block.digest||mapping?.kind!==view.output?.chart?.kind)return null;
      view.output.chart.mapping=clone(mapping);
    }
    if(view.output){
      view.output.retained_digest='synthetic-retained-'+output;
      // A separate synthetic disclosure projection exercises the renderer's
      // existing completeness contract. It is not evidence from the source test.
      if(output==='kpi-main')view.output.amount_completeness=[disclosure()];
    }
    if(!view.output?.table)view.page_bounds.limit=limit;
    return view;
  };
  window.startPublishedConsumerFixture=()=>{
    // This is a separate immutable published fixture, built only from published
    // baseline components. It does not promote or relabel the private draft.
    Object.assign(window,{consumerOnly:true,publishedPagesCatalog:true,pagesCatalog:false,compactCatalog:false,datasetMode:false,viewDelay:0,denyPrivate:true,denial:'',pageVisualValues:true});
    const d=clone(publishedDefinition),main={id:'main',title:'Summary',widgets:d.widgets,filters:d.filters||[]};
    delete d.widgets;delete d.filters;delete d.defaults;d.schema_version=3;
    const heading={id:'published-detail-heading',kind:'text',grid:{column:0,row:0,width:6,height:1},presentation:{},text:{format:'plain',text:'Published detail page'}};
    const table=clone(main.widgets.find(w=>w.block?.outputs.includes('table-main')));table.id='published-detail-table';table.grid={column:6,row:2,width:6,height:4};table.presentation.title='Published revenue detail';
    d.report_pages=[main,{id:'published-details',title:'Details',widgets:[heading,table]},{id:'published-empty',title:'Empty notes',widgets:[]}];
    window.publishedPagesDefinition=clone(d);
    runDefinitions.set('published-pages',{definition:clone(d),revision:2,private:false});
  };
  const blockViews=new Map();
  window.syntheticPrivateBlocks=blockViews;
  const blockKey=(id,revision)=>id+'@'+revision;
  const approvedView=()=>({data_validation:'not_performed',block:{schema_version:2,state:{id:'approved-block',version:8,draft_revision:0,published_revision:7},revision:7,digest:'a'.repeat(64),execution_digest:'b'.repeat(64),private:false,parameters:[{name:'maximum',type:'integer',default:{literal:'10'}}],expected_schema:mappingSamples.columns.map(c=>({name:c.name,type:c.type})),outputs:[{id:'kpi-main',kind:'kpi',editable:true,mapping:clone(samples.kpi.output.chart.mapping)},{id:'trend-main',kind:'chart',editable:true,mapping:clone(samples.trend.output.chart.mapping)},{id:'table-main',kind:'table',editable:true,mapping:{version:3,kind:'table',columns:clone(mappingSamples.columns),bindings:{columns:['c0','c1']},order:[],options:{title:'',legend:{visible:true,position:'bottom'},label_max_runes:80},table:{columns:[{column:'c0',visible:true},{column:'c1',visible:true}],page_size:1,show_totals:true}}}]},output_columns:['kpi-main','trend-main','table-main'].map(output=>({output,columns:clone(mappingSamples.columns)}))});
  window.startPageAuthoringFixture=()=>{
    Object.assign(window,{saveDelay:0,viewDelay:0,rejectSave:false,denyPrivate:false,consumerOnly:false,publishedPagesCatalog:false,compactCatalog:false,denial:'',pagesCatalog:false,pageVisualValues:true,cleanPaletteTitles:true});
    state={id:'report-a',kind:'report',version:20,draft_revision:10,published_revision:2};definition=clone(publishedDefinition);
    definition.filters=[{label:'Summary row limit',parameter:{name:'filter_1',type:'integer',required:false,default:{literal:'10'}}}];
    definition.widgets.find(w=>w.id==='trend').bindings=[{filter:'filter_1',parameter:'maximum'}];
    blockViews.clear();blockViews.set(blockKey('approved-block',7),approvedView());
    window.pageFixtureBaseline=clone(definition);
  };
  function mappingDispatch(name,a){
    if(name==='chart_catalog')return wrap(mappingSamples.catalog);
    if(name==='reporting_authoring_block_read_v1'){const v=blockViews.get(blockKey(a.block,a.revision));return v?wrap(v):failure('not_found');}
    const previous=blockViews.get(blockKey(a.block,a.revision));if(!previous)return failure('not_found');
    if(a.expected_version!==previous.block.state.version||a.digest!==previous.block.digest)return failure('conflict');
    if(name==='reporting_authoring_block_validate_v1'){
      if(!previous.block.private)return failure('forbidden');const v=clone(previous),b=v.block;
      b.state.version++;b.validation={id:'synthetic-validation-'+b.revision,revision:b.revision,definition_digest:b.digest,execution_digest:b.execution_digest,schema_digest:'e'.repeat(64),schema:clone(b.expected_schema),created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'};
      blockViews.set(blockKey(a.block,a.revision),v);return wrap({state:b.state,evidence:b.validation});
    }
    const copy=name==='reporting_authoring_block_copy_v1';
    if(copy&&(a.new_block!=='private-canvas-trend'||blockViews.has(blockKey(a.new_block,1)))||!copy&&(!previous.block.private||previous.block.state.draft_revision!==a.revision))return failure('forbidden');
    const v=clone(previous),b=v.block,id=copy?a.new_block:a.block,revision=copy?1:a.revision+1;
    b.state={id,version:copy?1:previous.block.state.version+1,draft_revision:revision,published_revision:0};b.revision=revision;b.private=true;b.digest=String(revision).repeat(64);b.execution_digest='f'.repeat(64);delete b.validation;
    const output=b.outputs.find(o=>o.id===a.output);if(!output?.editable)return failure('invalid_request');
    output.mapping={version:a.mapping.kpi||a.mapping.table?3:1,...clone(a.mapping),columns:clone(mappingSamples.columns)};output.kind=a.mapping.kind==='kpi'?'kpi':a.mapping.kind==='table'?'table':'chart';
    blockViews.set(blockKey(id,revision),v);return wrap(v);
  }
  // Dataset scenario consumes the separately captured safe native public DTOs.
  // Preparation/create metadata is exact; no generic mapping mock is used here.
  window.datasetEvidence=clone(datasetSamples||{});
  window.startDatasetAuthoringFixture=()=>{
    Object.assign(window,{datasetMode:true,datasetRetainedMode:false,datasetCreateUnknown:false,datasetCreateEffects:0,datasetPreparation:null,saveDelay:0,viewDelay:0,rejectSave:false,denyPrivate:false,consumerOnly:false,publishedPagesCatalog:false,compactCatalog:false,pagesCatalog:false,pageVisualValues:false,denial:''});
    state=clone(datasetEvidence.initial_read.state);definition=clone(datasetEvidence.initial_read.definition);
    window.datasetExecuted=false;window.datasetAdmitted=false;
    blockViews.clear();
  };
  const stable=value=>Array.isArray(value)?value.map(stable):value&&typeof value==='object'?Object.fromEntries(Object.keys(value).sort().map(k=>[k,stable(value[k])])):value;
  const same=(a,b)=>JSON.stringify(stable(a))===JSON.stringify(stable(b));window.same= same;
  function datasetDispatch(name,a){
    const f=datasetEvidence,key=blockKey(f.created.block.state.id,1);
    if(name==='list_topics')return wrap(f.list_topics);
    if(name==='describe_topic')return a.topic===f.dataset.topic.topic?wrap(f.describe_topic):failure('not_found');
    if(name==='reporting_authoring_dataset_v1')return same(a.topic,f.dataset.topic)&&a.dataset===f.dataset.dataset?wrap(f.dataset):failure('not_found');
    if(name==='reporting_authoring_prepare_chart_v1'){
      const expected=f.request;if(datasetPreparation)return failure('conflict');
      if(a.new_block!==expected.new_block||a.operation!==expected.operation||!same(a.intent,expected.intent)||a.metadata?.length!==1||['locale','title','question'].some(k=>a.metadata[0][k]!==expected.metadata[0][k]))return failure('invalid_request');
      datasetPreparation=clone(f.preparation);return wrap(datasetPreparation);
    }
    if(name==='reporting_authoring_preparation_v1')return datasetPreparation&&a.new_block===f.request.new_block&&(a.preparation===datasetPreparation.preparation||a.operation===datasetPreparation.operation)?wrap(datasetPreparation):failure('not_found');
    if(name==='reporting_authoring_create_prepared_v1'){
      if(!datasetPreparation||a.new_block!==datasetPreparation.new_block||a.preparation!==datasetPreparation.preparation||a.digest!==datasetPreparation.digest)return failure('stale_validation');
      if(!blockViews.has(key)){blockViews.set(key,clone(f.created));datasetCreateEffects++;}datasetPreparation.status='consumed';
      if(datasetCreateUnknown){datasetCreateUnknown=false;return {isError:true,structuredContent:{error:{code:'unavailable',outcome:'unknown'}}};}
      return wrap(blockViews.get(key));
    }
    if(name==='reporting_authoring_block_read_v1')return a.block===f.created.block.state.id&&a.revision===1&&blockViews.has(key)?wrap(blockViews.get(key)):failure('not_found');
    if(name==='reporting_authoring_block_validate_v1'){
      const v=blockViews.get(key);if(!v||a.block!==v.block.state.id||a.expected_version!==v.block.state.version||a.revision!==1||a.digest!==v.block.digest||!same(a.arguments,[]))return failure('stale_validation');
      v.block.state=clone(f.validation.state);v.block.validation=clone(f.validation.evidence);return wrap(f.validation);
    }
    return failure('forbidden');
  }
  const caps=report=>({version:'report-authoring-v1',consumer:true,builder:!consumerOnly,can_create:!consumerOnly&&['new-report','report-a'].includes(report),can_open:!consumerOnly&&!!report,can_save:!consumerOnly&&!!report,can_preview:!consumerOnly&&!!report,can_execute:!consumerOnly&&!!report});
  window.dispatch=(name,a)=>{
    // UI capability hints are not enforcement. This read-only host profile also
    // denies draft operations, execution and every exact private retained run.
    if(consumerOnly&&(name==='reporting_run'||name.startsWith('reporting_authoring_')&&name!=='reporting_authoring_capabilities_v1'))return failure('forbidden');
    if(consumerOnly&&name==='reporting_view'&&(privateRuns.has(a.run)||runDefinitions.get(a.run)?.private))return failure('not_found');

    if(window.datasetMode&&['list_topics','describe_topic','reporting_authoring_dataset_v1','reporting_authoring_prepare_chart_v1','reporting_authoring_preparation_v1','reporting_authoring_create_prepared_v1','reporting_authoring_block_read_v1','reporting_authoring_block_validate_v1'].includes(name))return datasetDispatch(name,a);
    if(window.datasetMode&&name==='reporting_authoring_read_v1')return a.report===state.id?wrap(state.draft_revision===datasetEvidence.initial_read.revision?datasetEvidence.initial_read:datasetEvidence.final_read):failure('not_found');
    if(window.datasetMode&&name==='reporting_authoring_save_v1'){
      if(!same(a,datasetEvidence.save_request)||state.version!==datasetEvidence.initial_read.state.version)return failure('conflict');
      state=clone(datasetEvidence.final_read.state);definition=clone(datasetEvidence.final_read.definition);return wrap(state);
    }
    if(window.datasetMode&&name==='reporting_authoring_preview_v1'){
      const f=datasetEvidence,b=blockViews.get(blockKey(f.created.block.state.id,1));
      if(a.report!==f.final_read.state.id||a.revision!==f.final_read.revision||!b?.block.validation||state.draft_revision!==f.final_read.revision||datasetAdmitted)return failure('forbidden');
      datasetAdmitted=true;return wrap(f.preview);
    }
    if(window.datasetMode&&name==='reporting_authoring_execute_v1'){
      if(!datasetAdmitted||a.run!==datasetEvidence.preview.id||a.resume!==false||datasetExecuted)return failure('forbidden');datasetExecuted=true;return wrap(datasetEvidence.complete);
    }
    if(window.datasetMode&&name==='reporting_view'){
      if(!datasetExecuted||a.kind!=='report'||a.run!==datasetEvidence.view_root.summary.run||a.offset!==0||a.limit!==100)return failure('not_found');
      if(a.page===''&&a.widget===''&&a.output==='')return wrap(datasetEvidence.view_root);
      return same(a,datasetEvidence.view_output.selection)?wrap(datasetEvidence.view_output):failure('not_found');
    }
    if(window.datasetMode&&name==='reporting_search')return wrap({version:'reporting-view-v1',items:[{target:{kind:'report',id:state.id,revision:1},title:definition.metadata.find(m=>m.locale==='en-US').title,description:'Native PostgreSQL synthetic-source fixture; no production provider connection',locale:'en-US'}],next:''});
    if(name==='chart_catalog'||['reporting_authoring_block_read_v1','reporting_authoring_block_copy_v1','reporting_authoring_block_mapping_v1','reporting_authoring_block_validate_v1'].includes(name))return mappingDispatch(name,a);
    if(name==='reporting_authoring_capabilities_v1')return wrap(caps(a.report));
    if(name==='reporting_search')return wrap({version:'reporting-view-v1',items:a.kind==='block'?[{target:{kind:'block',id:'approved-block',revision:7},title:'Approved business metrics',description:'Synthetic approved outputs',locale:'en-US'}]:[{target,title:'Weekly operations',description:'Synthetic retained KPI, trend and operational detail',locale:'en-US'}],next:''});
    if(name==='reporting_authoring_drafts_v1')return wrap({items:[{id:state.id,version:state.version,revision:state.draft_revision,metadata:definition.metadata,updated_at:'2026-10-03T12:00:00Z'}],next:''});
    if(name==='reporting_authoring_read_v1')return wrap({state,revision:state.draft_revision,private:true,digest:'synthetic',definition});
    if(name==='reporting_authoring_create_v1'){state={id:a.id,kind:'report',version:1,draft_revision:1,published_revision:0};definition=clone(a.definition);return wrap(state);}
    if(name==='reporting_authoring_save_v1'){if(rejectSave){rejectSave=false;return failure('conflict');}if(a.expected_version!==state.version)return failure('conflict');state={...state,version:state.version+1,draft_revision:state.draft_revision+1};definition=clone(a.definition);return wrap(state);}
    if(name==='reporting_describe')return wrap(a.target.kind==='block'?{version:'reporting-view-v1',resource:{target:a.target,title:'Approved business metrics'},outputs:[{id:'kpi-main',kind:'kpi',title:outputTitles['kpi-main'],enabled:true,selected:true},{id:'trend-main',kind:'chart',title:outputTitles['trend-main'],enabled:true,selected:true},{id:'table-main',kind:'table',title:window.cleanPaletteTitles?outputTitles['table-main']:'<img src=x onerror=alert(1)>',enabled:true,selected:true},{id:'story',kind:'narrative',title:'Narrative',enabled:true,selected:true}],filters:[filter],dynamic:false,timezone:'UTC'}:{version:'reporting-view-v1',resource:{target,title:'Weekly operations'},outputs:[],filters:[filter],pages:[],dynamic:false,timezone:'UTC'});
    if(name==='reporting_runs'&&window.publishedPagesCatalog)return wrap({version:'reporting-view-v1',items:[{kind:'report',run:'published-pages',target:{...target},state:'completed',private:false,created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'}],next:''});
    if(name==='reporting_runs'&&window.pagesCatalog&&consumerOnly)return wrap({version:'reporting-view-v1',items:[],next:''});
    if(name==='reporting_runs'&&window.pagesCatalog)return wrap({version:'reporting-view-v1',items:[{kind:'report',run:'private-one',target:{...target,revision:state.draft_revision},state:'completed',private:true,created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'}],next:''});
    if(name==='reporting_runs')return wrap({version:'reporting-view-v1',items:[{kind:'report',run:compactCatalog?'retained-compact':'retained-one',target:{...target,revision:compactCatalog?1:2},state:'completed',private:false,created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'}],next:''});
    if(name==='reporting_run'){runDefinitions.set('new-run',{definition:clone(publishedDefinition),revision:2,private:false});return wrap({version:'reporting-view-v1',kind:'report',run:'new-run',state:'completed',code:'',target});}
    if(name==='reporting_authoring_preview_v1'){privateRuns.add('private-one');runDefinitions.set('private-one',{definition:clone(definition),report:a.report,revision:a.revision,private:true});return wrap({id:'private-one',kind:'report',document:a.report,revision:a.revision,private:true,state:'admitted',pages:[]});}
    if(name==='reporting_authoring_execute_v1')return wrap({id:a.run,kind:'report',document:state.id,revision:state.draft_revision,private:true,state:'completed',pages:[]});
    if(name==='reporting_view'){
      if(denyPrivate&&privateRuns.has(a.run))return failure('not_found');
      if(denial==='context'&&a.offset===1)return failure('forbidden');
      const view=makeView(a);if(!view)return failure('not_found');
      if(denial==='target'&&a.offset===1||denial==='initial-target'&&!a.widget)view.summary.target.id='another-report';
      if(denial==='initial-revision'&&!a.widget)view.summary.target.revision=99;
      if(denial==='run'&&a.offset===1){view.summary.run='another-run';view.selection.run='another-run';}
      return wrap(view);
    }
    return failure('forbidden');
  };
  frame.addEventListener('load',()=>{if(embedded)frame.contentWindow.postMessage({protocol:'chartworks-report-app-v1',method:'bootstrap',frame:'fixture-frame',generation:1,params:{challenge:'fixture-challenge-0001'}},location.origin);});
  window.addEventListener('message',event=>{
    if(event.source!==frame.contentWindow||event.origin!==location.origin)return;
    const m=event.data;if(embedded?m?.protocol!=='chartworks-report-app-v1':m?.jsonrpc!=='2.0')return;
    const send=result=>frame.contentWindow.postMessage(embedded?{protocol:'chartworks-report-app-v1',frame:'fixture-frame',generation:1,id:m.id,result}:{jsonrpc:'2.0',id:m.id,result},location.origin);
    if(m.method==='initialize'&&embedded){send({challenge:'fixture-challenge-0001',tools:true,context:{theme:'light',locale:'en-US'}});return;}
    if(m.method==='ui/initialize'&&!embedded){send({protocolVersion:'2026-01-26',hostCapabilities:{serverTools:{}},hostContext:{theme:'light',locale:'en-US'}});return;}
    if(m.method==='ui/notifications/size-changed'){window.lastResizeAt=performance.now();window.resizeCount++;window.resizeMessages.push(clone(m.params));frame.style.height=Math.min(2416,Math.max(500,Math.ceil(m.params.height)+16))+'px';return;}
    if(m.method==='tools/call'){calls.push(clone(m.params));const result=dispatch(m.params.name,m.params.arguments);setTimeout(()=>send(result),m.params.name==='reporting_authoring_save_v1'?saveDelay:m.params.name==='reporting_view'?viewDelay||5:5);}
  });
  window.closeApp=()=>frame.contentWindow.postMessage(embedded?{protocol:'chartworks-report-app-v1',frame:'fixture-frame',generation:1,method:'close'}:{jsonrpc:'2.0',id:900,method:'ui/resource-teardown'},location.origin);
}
