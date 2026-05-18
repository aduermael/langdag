# Deployment-Aware Model Catalog

Langdag publishes a deployment-aware model catalog as a static JSON artifact.
The catalog separates model ownership from the API surface used to call a model
and the deployment that hosts it.

## Source Of Truth

The catalog JSON is committed only on the dedicated publishing branch:

`origin/model-catalog`

That branch contains the source-of-truth file:

`docs/model-catalog/v1/catalog.json`

GitHub Pages serves that file at:

`https://langdag.com/model-catalog/v1/catalog.json`

The catalog file is intentionally not committed to `main` or feature branches.
Normal website deploys upload the docs tree from `main`, then overlay the
catalog from `origin/model-catalog` when that branch exists. This keeps a
website redeploy from rolling the catalog back to an older copy.

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
Ollama IDs are discovered. Azure OpenAI requires deployment-scoped model
mappings because the user's Azure deployment name is the route path segment.
