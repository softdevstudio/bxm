"""The CanonError class, defined in Python so it pickles cleanly and always
has `code` and `offset`."""


class CanonError(ValueError):
    """An error from Canon.

    `code` is a stable short name such as "overlong-varint" or
    "keys-out-of-order" ("text" for text that does not parse), and `offset` is
    the byte where the problem was found, or -1.
    """

    code = "error"
    offset = -1

    def __init__(self, message="", code="error", offset=-1):
        super().__init__(message)
        self.code = code
        self.offset = offset

    def __reduce__(self):
        return (type(self), (str(self), self.code, self.offset))
