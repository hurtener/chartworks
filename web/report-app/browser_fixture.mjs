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

// Stringified into the isolated test host. All state is disposable and belongs to
// this fixture; the compiled application receives only normal tool DTOs.
export function initializeSyntheticHost(samples, embedded) {
  const frame=document.getElementById('app');
  const clone=value=>JSON.parse(JSON.stringify(value));
  const wrap=value=>({structuredContent:{result:clone(value)}});
  const failure=code=>({isError:true,structuredContent:{error:{code,outcome:'not_started'}}});
  Object.assign(window,{calls:[],lastResizeAt:0,resizeCount:0,resizeMessages:[],saveDelay:0,viewDelay:0,rejectSave:false,denyPrivate:false,consumerOnly:false,compactCatalog:false,denial:'',privateRuns:new Set(),waiting:[]});
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
  const sampleFor=output=>output==='kpi-main'?samples.kpi:output==='trend-main'?samples.trend:samples.table;
  const disclosure=()=>({label:'Synthetic known revenue',evidence:'reviewed_definition',definition_digest:'b'.repeat(64),declaration:'synthetic-known-revenue',value_field:'amount',unknown_count_field:'unknown_amount_count',query_outcome:'succeeded',rows_scope:'returned_query_rows',role:'amount',unit:'USD',result:{policy:'reviewed-amount-completeness-v1',scope:'returned_query_rows',metric:'synthetic:known_revenue',value_column:1,unknown_count_metric:'synthetic:unknown_amount_count',unknown_count_column:2,status:'incomplete',rows:[{row:0,status:'incomplete',unknown_count:'2'}]}});
  window.makeView=request=>{
    const run=runDefinitions.get(request.run);if(!run)return null;
    const d=run.definition,page=request.page||'main',widget=d.widgets.find(w=>w.id===(request.widget||d.widgets[0].id));
    if(page!=='main'||!widget)return null;
    const output=request.output||(widget.block?.outputs[0]||'');
    if(widget.kind==='text'?output!=='':!widget.block.outputs.includes(output))return null;
    const source=widget.kind==='text'?samples.heading:output==='table-main'&&request.offset===1?samples.tableNext:sampleFor(output);
    const view=clone(source),limit=request.limit||100;
    view.summary={...view.summary,kind:'report',run:request.run,target:{...target,revision:run.revision},private:run.private,state:'completed',created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'};
    view.selection={kind:'report',run:request.run,page,widget:widget.id,output,offset:request.offset||0,limit};
    view.locale=d.locale;view.timezone=d.timezone;view.filters=[filter];
    view.pages=[{id:'main',title:'Synthetic report canvas',report:'report-a',revision:run.revision,widgets:d.widgets.map(w=>({id:w.id,kind:w.kind,state:'completed',grid:clone(w.grid),presentation:clone(w.presentation),outputs:w.block?clone(w.block.outputs):[]}))}];
    view.outputs=widget.block?widget.block.outputs.map(id=>({id,kind:sampleFor(id).output.kind,title:outputTitles[id],enabled:true,selected:true})):[];
    if(widget.kind==='text')view.text=clone(widget.text);
    if(view.output){
      view.output.retained_digest='synthetic-retained-'+output;
      // A separate synthetic disclosure projection exercises the renderer's
      // existing completeness contract. It is not evidence from the source test.
      if(output==='kpi-main')view.output.amount_completeness=[disclosure()];
    }
    if(!view.output?.table)view.page_bounds.limit=limit;
    return view;
  };
  const caps=report=>({version:'report-authoring-v1',consumer:true,builder:!consumerOnly,can_create:!consumerOnly&&['new-report','report-a'].includes(report),can_open:!consumerOnly&&!!report,can_save:!consumerOnly&&!!report,can_preview:!consumerOnly&&!!report,can_execute:!consumerOnly&&!!report});
  window.dispatch=(name,a)=>{
    if(name==='reporting_authoring_capabilities_v1')return wrap(caps(a.report));
    if(name==='reporting_search')return wrap({version:'reporting-view-v1',items:a.kind==='block'?[{target:{kind:'block',id:'approved-block',revision:7},title:'Approved business metrics',description:'Synthetic approved outputs',locale:'en-US'}]:[{target,title:'Weekly operations',description:'Synthetic retained KPI, trend and operational detail',locale:'en-US'}],next:''});
    if(name==='reporting_authoring_drafts_v1')return wrap({items:[{id:state.id,version:state.version,revision:state.draft_revision,metadata:definition.metadata,updated_at:'2026-10-03T12:00:00Z'}],next:''});
    if(name==='reporting_authoring_read_v1')return wrap({state,revision:state.draft_revision,private:true,digest:'synthetic',definition});
    if(name==='reporting_authoring_create_v1'){state={id:a.id,kind:'report',version:1,draft_revision:1,published_revision:0};definition=clone(a.definition);return wrap(state);}
    if(name==='reporting_authoring_save_v1'){if(rejectSave){rejectSave=false;return failure('conflict');}if(a.expected_version!==state.version)return failure('conflict');state={...state,version:state.version+1,draft_revision:state.draft_revision+1};definition=clone(a.definition);return wrap(state);}
    if(name==='reporting_describe')return wrap(a.target.kind==='block'?{version:'reporting-view-v1',resource:{target:a.target,title:'Approved business metrics'},outputs:[{id:'kpi-main',kind:'kpi',title:outputTitles['kpi-main'],enabled:true,selected:true},{id:'trend-main',kind:'chart',title:outputTitles['trend-main'],enabled:true,selected:true},{id:'table-main',kind:'table',title:'<img src=x onerror=alert(1)>',enabled:true,selected:true},{id:'story',kind:'narrative',title:'Narrative',enabled:true,selected:true}],filters:[filter],dynamic:false,timezone:'UTC'}:{version:'reporting-view-v1',resource:{target,title:'Weekly operations'},outputs:[],filters:[filter],pages:[],dynamic:false,timezone:'UTC'});
    if(name==='reporting_runs')return wrap({version:'reporting-view-v1',items:[{kind:'report',run:compactCatalog?'retained-compact':'retained-one',target:{...target,revision:compactCatalog?1:2},state:'completed',private:false,created_at:'2026-10-03T12:00:00Z',expires_at:'2099-01-01T00:00:00Z'}],next:''});
    if(name==='reporting_run'){runDefinitions.set('new-run',{definition:clone(publishedDefinition),revision:2,private:false});return wrap({version:'reporting-view-v1',kind:'report',run:'new-run',state:'completed',code:'',target});}
    if(name==='reporting_authoring_preview_v1'){privateRuns.add('private-one');runDefinitions.set('private-one',{definition:clone(definition),revision:a.revision,private:true});return wrap({id:'private-one',kind:'report',document:a.report,revision:a.revision,private:true,state:'admitted',pages:[]});}
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
