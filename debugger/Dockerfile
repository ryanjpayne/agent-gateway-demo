# Stage 1: Build the proto libraries from the envoy git repo using buf.
FROM bufbuild/buf as BUF
RUN apk add git

# L7 proto path: envoy/service/ext_proc/v3/external_processor.proto
ARG proto_path="envoy/service/ext_proc/v3/external_processor.proto"

# Copy the Buf generation configuration
COPY buf.gen.yaml .

# Generate the protobuf and gRPC python stubs from Envoy's official API repository
RUN buf -v generate https://github.com/envoyproxy/envoy.git#subdir=api --path $proto_path --include-imports

# Stage 2: Build the runner image
FROM python:3.13-slim

WORKDIR /app

# Install standard Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy over the generated Envoy protobuf files from the BUF build stage
COPY --from=BUF /protodef /app

# Copy the service extension code
COPY interceptor.py .

# Make port 50051 available
EXPOSE 50051

# Run the interceptor
CMD ["python", "interceptor.py"]
