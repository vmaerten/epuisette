class ApiError(Exception):
    """A failure sent to the client as `{code, message}` with the given HTTP status."""

    def __init__(self, status: int, code: str, message: str):
        super().__init__(message)
        self.status = status
        self.code = code
        self.message = message
