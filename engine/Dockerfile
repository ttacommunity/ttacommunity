# The engine image for local compose. It has the same layout as the image
# scripts/engine-image.sh assembles with crane: wheels in /engine/deps.
FROM ghcr.io/astral-sh/uv:0.12.19@sha256:04d046b13e60d6bcec73cbc5e1cad25d680dea90c8573340950a0ac2d1aef424 AS uv

FROM docker.io/library/python:3.12-slim@sha256:f77ac9e44ae96ef2c90b8053ea08c31f8be030f824196b0ae4db6d462c84e51f
COPY --from=uv /uv /usr/local/bin/uv
WORKDIR /engine
COPY pyproject.toml uv.lock ./
RUN uv export --frozen --no-dev --no-hashes --no-emit-project -o /tmp/requirements.txt && \
    pip install --no-cache-dir --only-binary=:all: --target /engine/deps -r /tmp/requirements.txt && \
    rm /usr/local/bin/uv /tmp/requirements.txt
COPY . .
ENV PYTHONPATH=/engine/deps:/engine/src PYTHONDONTWRITEBYTECODE=1
USER 65532:65532
ENTRYPOINT ["python"]
CMD ["-c", "import numpy; print('engine image ok')"]
