"""LangDAG SDK data models."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from enum import Enum
from typing import Any


class NodeType(str, Enum):
    """Type of a node."""

    USER = "user"
    ASSISTANT = "assistant"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"


class SSEEventType(str, Enum):
    """Type of Server-Sent Event."""

    START = "start"
    DELTA = "delta"
    DONE = "done"
    ERROR = "error"


@dataclass
class ToolDefinition:
    """Definition of a tool that can be used by the LLM."""

    name: str
    description: str | None = None
    input_schema: dict[str, Any] | None = None


@dataclass
class SSEEvent:
    """A Server-Sent Event from a streaming response."""

    event: SSEEventType
    data: dict[str, Any]

    @property
    def node_id(self) -> str | None:
        """Get the node ID from done events."""
        return self.data.get("node_id")

    @property
    def content(self) -> str | None:
        """Get the content from delta events."""
        return self.data.get("content")


@dataclass
class NormalizedUsage:
    """Provider-normalized billable usage dimensions."""

    input_tokens: int | None = None
    output_tokens: int | None = None
    cache_read_input_tokens: int | None = None
    cache_creation_input_tokens: int | None = None
    cache_write_input_tokens: int | None = None
    reasoning_tokens: int | None = None
    tool_use_prompt_tokens: int | None = None
    audio_input_tokens: int | None = None
    audio_output_tokens: int | None = None
    image_input_tokens: int | None = None
    image_output_tokens: int | None = None
    accepted_prediction_tokens: int | None = None
    rejected_prediction_tokens: int | None = None
    service_tier: str | None = None
    dimensions: dict[str, int] | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> NormalizedUsage | None:
        if not data:
            return None
        return cls(
            input_tokens=data.get("input_tokens"),
            output_tokens=data.get("output_tokens"),
            cache_read_input_tokens=data.get("cache_read_input_tokens"),
            cache_creation_input_tokens=data.get("cache_creation_input_tokens"),
            cache_write_input_tokens=data.get("cache_write_input_tokens"),
            reasoning_tokens=data.get("reasoning_tokens"),
            tool_use_prompt_tokens=data.get("tool_use_prompt_tokens"),
            audio_input_tokens=data.get("audio_input_tokens"),
            audio_output_tokens=data.get("audio_output_tokens"),
            image_input_tokens=data.get("image_input_tokens"),
            image_output_tokens=data.get("image_output_tokens"),
            accepted_prediction_tokens=data.get("accepted_prediction_tokens"),
            rejected_prediction_tokens=data.get("rejected_prediction_tokens"),
            service_tier=data.get("service_tier"),
            dimensions=data.get("dimensions"),
        )


@dataclass
class ModelResolutionMetadata:
    """Resolved model/deployment identity for an assistant response."""

    canonical_model_id: str | None = None
    offering_id: str | None = None
    deployment_id: str | None = None
    provider_id: str | None = None
    api_protocol_id: str | None = None
    native_model_id: str | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> ModelResolutionMetadata | None:
        if not data:
            return None
        return cls(**{k: data.get(k) for k in cls.__dataclass_fields__})


@dataclass
class PricingSnapshot:
    """Catalog pricing copied onto a saved assistant response."""

    status: str | None = None
    currency: str | None = None
    effective_at: str | None = None
    source: str | None = None
    rates_per_1m: dict[str, float] | None = None
    missing_dimensions: list[str] | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> PricingSnapshot | None:
        if not data:
            return None
        return cls(
            status=data.get("status"),
            currency=data.get("currency"),
            effective_at=data.get("effective_at"),
            source=data.get("source"),
            rates_per_1m=data.get("rates_per_1m"),
            missing_dimensions=data.get("missing_dimensions"),
        )


@dataclass
class ProviderCost:
    """Exact cost reported synchronously by a provider, when available."""

    total: float
    currency: str
    source: str
    raw: Any | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> ProviderCost | None:
        if not data:
            return None
        return cls(total=data["total"], currency=data["currency"], source=data["source"], raw=data.get("raw"))


@dataclass
class CostResult:
    """Structured cost calculation result."""

    status: str
    total: float | None = None
    currency: str | None = None
    source: str | None = None
    missing_dimensions: list[str] | None = None
    dimensions: list[dict[str, Any]] | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> CostResult | None:
        if not data:
            return None
        return cls(
            status=data["status"],
            total=data.get("total"),
            currency=data.get("currency"),
            source=data.get("source"),
            missing_dimensions=data.get("missing_dimensions"),
            dimensions=data.get("dimensions"),
        )


@dataclass
class AssistantNodeMetadata:
    """Typed assistant-node metadata stored in the API metadata field."""

    model_resolution: ModelResolutionMetadata | None = None
    normalized_usage: NormalizedUsage | None = None
    pricing_snapshot: PricingSnapshot | None = None
    provider_cost: ProviderCost | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any] | None) -> AssistantNodeMetadata | None:
        if not data:
            return None
        return cls(
            model_resolution=ModelResolutionMetadata.from_dict(data.get("model_resolution")),
            normalized_usage=NormalizedUsage.from_dict(data.get("normalized_usage")),
            pricing_snapshot=PricingSnapshot.from_dict(data.get("pricing_snapshot")),
            provider_cost=ProviderCost.from_dict(data.get("provider_cost")),
        )


@dataclass
class Node:
    """A node in a conversation tree."""

    id: str
    sequence: int
    node_type: NodeType
    content: str
    created_at: datetime
    parent_id: str | None = None
    root_id: str | None = None
    provider: str | None = None
    model: str | None = None
    tokens_in: int | None = None
    tokens_out: int | None = None
    cache_read_tokens_in: int | None = None
    cache_creation_tokens_in: int | None = None
    reasoning_tokens: int | None = None
    latency_ms: int | None = None
    stop_reason: str | None = None
    output_group_id: str | None = None
    status: str | None = None
    title: str | None = None
    system_prompt: str | None = None
    metadata: AssistantNodeMetadata | None = None
    cost: CostResult | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> Node:
        """Create a Node from a dictionary."""
        return cls(
            id=data["id"],
            sequence=data["sequence"],
            node_type=NodeType(data["node_type"]),
            content=data["content"],
            created_at=_parse_datetime(data["created_at"]),
            parent_id=data.get("parent_id"),
            root_id=data.get("root_id"),
            provider=data.get("provider"),
            model=data.get("model"),
            tokens_in=data.get("tokens_in"),
            tokens_out=data.get("tokens_out"),
            cache_read_tokens_in=data.get("tokens_cache_read"),
            cache_creation_tokens_in=data.get("tokens_cache_creation"),
            reasoning_tokens=data.get("tokens_reasoning"),
            latency_ms=data.get("latency_ms"),
            stop_reason=data.get("stop_reason"),
            output_group_id=data.get("output_group_id"),
            status=data.get("status"),
            title=data.get("title"),
            system_prompt=data.get("system_prompt"),
            metadata=AssistantNodeMetadata.from_dict(data.get("metadata")),
            cost=CostResult.from_dict(data.get("cost")),
        )


@dataclass
class PromptResponse:
    """Response from a prompt request."""

    node_id: str
    content: str
    tokens_in: int | None = None
    tokens_out: int | None = None
    tokens_cache_read: int | None = None
    tokens_cache_creation: int | None = None
    tokens_reasoning: int | None = None
    output_group_id: str | None = None
    usage: NormalizedUsage | None = None
    metadata: AssistantNodeMetadata | None = None
    cost: CostResult | None = None

    @classmethod
    def from_dict(cls, data: dict[str, Any]) -> PromptResponse:
        """Create a PromptResponse from a dictionary."""
        return cls(
            node_id=data["node_id"],
            content=data["content"],
            tokens_in=data.get("tokens_in"),
            tokens_out=data.get("tokens_out"),
            tokens_cache_read=data.get("tokens_cache_read"),
            tokens_cache_creation=data.get("tokens_cache_creation"),
            tokens_reasoning=data.get("tokens_reasoning"),
            output_group_id=data.get("output_group_id"),
            usage=NormalizedUsage.from_dict(data.get("usage")),
            metadata=AssistantNodeMetadata.from_dict(data.get("metadata")),
            cost=CostResult.from_dict(data.get("cost")),
        )


def _parse_datetime(value: str | datetime) -> datetime:
    """Parse a datetime string or return as-is if already a datetime."""
    if isinstance(value, datetime):
        return value
    # Handle ISO format with optional timezone
    if value.endswith("Z"):
        value = value[:-1] + "+00:00"
    return datetime.fromisoformat(value)
