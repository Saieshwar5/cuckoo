# The welcome agent as an image, so it runs beside the hub in production.
# Build from the repository root, because it needs the SDK:
#
#   docker build -t cuckoo-welcome:<version> -f examples/welcome/Dockerfile .

FROM python:3.12-slim
WORKDIR /app

COPY sdk/python /app/sdk
RUN pip install --no-cache-dir /app/sdk

COPY examples/welcome/welcome.py /app/welcome.py
RUN adduser --system --uid 10002 agent
USER agent

# CUCKOO_SECRET and CUCKOO_HUB come from the environment; see deploy/compose.yml.
CMD ["python", "-u", "welcome.py"]
