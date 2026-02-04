"""LangDAG SDK exceptions."""

from __future__ import annotations


class LangDAGError(Exception):
    """Base exception for LangDAG SDK errors."""

    pass


class APIError(LangDAGError):
    """Raised when the API returns an error response."""

    def __init__(
        self,
        message: str,
        status_code: int | None = None,
        response_body: dict | None = None,
    ) -> None:
        super().__init__(message)
        self.status_code = status_code
        self.response_body = response_body

    def __str__(self) -> str:
        if self.status_code:
            return f"[{self.status_code}] {super().__str__()}"
        return super().__str__()


class AuthenticationError(APIError):
    """Raised when authentication fails (401)."""

    pass


class NotFoundError(APIError):
    """Raised when a resource is not found (404)."""

    pass


class BadRequestError(APIError):
    """Raised when the request is invalid (400)."""

    pass


class ConnectionError(LangDAGError):
    """Raised when unable to connect to the API."""

    pass


class StreamError(LangDAGError):
    """Raised when there's an error during streaming."""

    pass
