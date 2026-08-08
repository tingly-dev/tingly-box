"""Generated models, produced from tingly-box's ``openapi.json``.

``models.py`` next to this file is written by ``task gen:py`` and is not
committed — it is a pure function of the spec, so the repository keeps the
spec and CI rebuilds the models before tests and before the wheel is built.
A wheel installed from PyPI therefore always contains it.

A source checkout does not, until you run the generator. That is the one
failure mode this module exists to explain: without it, importing ``tingly``
would fail with a bare ``ModuleNotFoundError: tingly._generated.models``,
which says nothing about what to do next.
"""

from __future__ import annotations

MISSING_MODELS_HINT = """\
tingly's generated models are missing.

They are built from tingly-box's openapi.json and deliberately not committed.
From the repository root, run:

    task gen:py

(or, without go-task:
    pip install 'datamodel-code-generator>=0.72'
    python3 -m datamodel_code_generator --input openapi.json \\
      --input-file-type openapi --output-model-type pydantic_v2.BaseModel \\
      --target-python-version 3.10 \\
      --output sdk/python/tingly/_generated/models.py
)

If you hit this in an installed package rather than a checkout, the wheel was
built without running the generator — please report it."""


# Importing `tingly._generated.models` imports this package first, so this is
# the one place that can turn a bare "No module named tingly._generated.models"
# into something that says what to run. Doing it here rather than in each
# consumer means every import path — ours and a user's — gets the same message.
#
# Checked with find_spec rather than a try/import: while this package is still
# initializing, `from . import models` on a missing submodule surfaces as
# ImportError("partially initialized module"), which would swallow the real
# cause behind a misleading circular-import message.
def _require_models() -> None:
    from importlib.util import find_spec

    try:
        found = find_spec(__name__ + ".models") is not None
    except (ImportError, ValueError):
        found = False
    if not found:
        raise ModuleNotFoundError(MISSING_MODELS_HINT, name=__name__ + ".models")


_require_models()
del _require_models
