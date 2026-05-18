# Deployment-Aware Model Catalog

Langdag publishes a deployment-aware model catalog as a static JSON artifact.
The catalog separates model ownership from the API surface used to call a model
and the deployment that hosts it.

## Source Of Truth

The website-served catalog JSON is committed only on the dedicated publishing
branch:

`origin/model-catalog`

That branch contains the source-of-truth file:

`docs/model-catalog/v1/catalog.json`

GitHub Pages serves that file at:

`https://langdag.com/model-catalog/v1/catalog.json`

The website catalog file is intentionally not committed to `main` or feature
branches. Normal website deploys upload the docs tree from `main`, then overlay
the catalog from `origin/model-catalog` when that branch exists. This keeps a
website redeploy from rolling the catalog back to an older copy. The embedded
Go release artifact at `internal/models/catalog.json` is generated from that
same branch and committed so module release tags are self-contained.

## Automated Updates

The `Refresh Model Catalog` workflow runs every hour. It:

1. Generates a candidate catalog from provider documentation.
2. Validates the candidate with the model catalog tests.
3. Compares the candidate to the current published catalog while ignoring
   generation-time fields such as `generated_at`, `stale_after`,
   `provenance.observed_at`, and `pricing.effective_at`.
4. Commits the JSON artifact to `origin/model-catalog` only when real catalog
   content changed.
5. Deploys GitHub Pages from that updated docs tree.

The `model-catalog` branch is a publishing branch. It is not intended to be
merged into `main`; repository rules should restrict writes to the scheduled
workflow's GitHub App publisher.

## Contract

Catalog v1 uses `schema_version: "model-catalog/v1"` and contains normalized
maps for providers, API protocols, deployments, canonical models, offerings, and
mapping-required offering templates.

Pricing status is one of `known`, `partial`, `unknown`, or `free`. Unknown
pricing remains distinct from zero-dollar pricing. Capability state is one of
`supported`, `unsupported`, or `unknown`.

Remote catalog data is data-only. It can add known models, offerings, prices,
capabilities, aliases, and provenance for known deployments, but it cannot define
arbitrary endpoints, auth flows, request templates, or new protocol behavior.

## Runtime Loading

Prompt/runtime loading uses the embedded catalog generated from
`origin/model-catalog`. Maintainers update the committed
`internal/models/catalog.json` artifact manually by running
`./scripts/sync-model-catalog.sh` before cutting a release.

LangDAG does not implicitly read `~/.config/langdag/model_catalog.json`, so a
stale user cache cannot override the published catalog snapshot. Apps that want
the freshest catalog at startup can set `RemoteModelCatalog`; apps that want a
one-shot fetch can call `LoadRemoteModelCatalog` or `RefreshModelCatalogCache`
with no cache path and pass the returned catalog via `Config.ModelCatalog`.
`langdag models --update` can fetch the published artifact for one command.
`LANGDAG_MODEL_CATALOG_URL` overrides the CLI endpoint and
`LANGDAG_MODEL_CATALOG_TIMEOUT` overrides the CLI fetch timeout. Remote data is
strictly schema-validated before use.

## Current Adapter Mapping

| Langdag provider variant | Provider | API protocol | Deployment |
| --- | --- | --- | --- |
| `anthropic` | `anthropic` | `anthropic-messages` | `anthropic-direct` |
| `anthropic-bedrock` | `anthropic` | `anthropic-messages` | `anthropic-bedrock` |
| `anthropic-vertex` | `anthropic` | `anthropic-messages` | `anthropic-vertex` |
| `openai` | `openai` | `openai-chat-completions` | `openai-direct` |
| `openai-azure` | `openai` | `openai-chat-completions` | `openai-azure` |
| `gemini` / `gemma` | `google` | `gemini-generate-content` | `gemini-direct` |
| `gemini-vertex` | `google` | `gemini-generate-content` | `gemini-vertex` |
| `grok` | `xai` | `openai-responses` | `grok-direct` |
| `openrouter` | `openrouter` | `openai-chat-completions` | `openrouter` |
| `ollama` | `ollama` | `openai-chat-completions` | `ollama-local` |

## Native Model IDs

`native_model_id` is the exact value passed to the selected adapter. Direct
provider deployments usually use catalog-known native IDs. OpenRouter and
Ollama IDs are discovered. Azure OpenAI requires deployment-scoped
`model_mappings` because the user's Azure deployment name is the route path
segment. Static catalog data records an Azure offering template, not a routeable
offering; resolution materializes the real offering ID and `native_model_id`
only after config supplies the exact mapping value.

## Routing And Fallback

Callers target canonical model IDs. The deployment router resolves that
canonical model to an eligible offering, rewrites the request to the selected
deployment's `native_model_id`, calls the adapter, and attaches served identity
and pricing metadata to the response.

Scoped routing is selected in this order:

1. `routing.models[canonical_model_id]`
2. `routing.providers[provider_id]`
3. `routing.default`, only when explicitly configured

Model and provider overrides apply only to matching canonical models. If a
matching override has no eligible deployment, langdag reports the failure
instead of appending default stages. Non-matching models keep using automatic
eligible deployment resolution unless `routing.default` is explicitly
configured as an advanced global baseline.
Each stage contains weighted deployments and a retry count. A stage skips
deployments that are not configured, cannot serve the canonical model, or
require a missing Azure `model_mappings` entry. When server tools are requested,
langdag prefers eligible deployments with known support; if none exist, it keeps
the route but strips unsupported or unknown server tools before calling the
adapter. The next explicit stage is tried only after the current stage's retries
fail.

Streaming fallback is conservative: langdag can retry or fall back before any
output has been emitted. After a stream sends output, any later error is
surfaced to the caller without switching deployments.

Advanced example with an explicit global default route:

```go
client, err := langdag.New(langdag.Config{
    Deployments: map[string]langdag.DeploymentConfig{
        "openai-direct": {APIKey: os.Getenv("OPENAI_API_KEY")},
        "openai-azure": {
            APIKey:     os.Getenv("AZURE_OPENAI_API_KEY"),
            Endpoint:   os.Getenv("AZURE_OPENAI_ENDPOINT"),
            APIVersion: "2024-08-01-preview",
            ModelMappings: map[string]string{
                "openai/gpt-4.1-2025-04-14": "gpt-41-prod",
            },
        },
        "openrouter": {APIKey: os.Getenv("OPENROUTER_API_KEY")},
    },
    RoutingPolicy: &langdag.RoutingPolicy{
        Providers: map[string][]langdag.RoutingStage{
            "openai": {{
                Deployments: []langdag.DeploymentChoice{
                    {DeploymentID: "openai-direct", Weight: 70},
                    {DeploymentID: "openai-azure", Weight: 30},
                },
                Retries: 1,
            }},
        },
        Default: []langdag.RoutingStage{{
            Deployments: []langdag.DeploymentChoice{{DeploymentID: "openrouter", Weight: 100}},
        }},
    },
})
```

Then target the canonical model:

```go
result, err := client.Prompt(ctx, "Summarize this diff",
    langdag.WithModel("openai/gpt-4.1-2025-04-14"),
)
```

## Cost Source Audit

The catalog is the upfront comparison and estimate source. For the currently
supported adapters, direct Anthropic, Bedrock, Vertex, OpenAI Chat Completions,
Azure OpenAI, Gemini, Gemini Vertex, and Grok preserve usage counters
synchronously. Ollama is local/free in catalog accounting. OpenRouter returns
synchronous usage counters through the current adapter; exact credit accounting,
when needed, requires an asynchronous generation lookup and is not the current
per-response source of truth.

Responses carry normalized usage dimensions and a pricing snapshot copied from
the served offering. If a provider returns exact per-response cost, that exact
cost is preserved with source `provider_response` and should be preferred over
catalog estimates for that response. Otherwise callers can compute structured
cost results from the saved usage and pricing snapshot, including `known`,
`partial`, `unknown`, and `free` statuses.
