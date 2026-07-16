import os
import grpc
from concurrent import futures
import json
import gzip

# Envoy external processing specific protobuf imports
from envoy.service.ext_proc.v3 import external_processor_pb2_grpc as ext_proc_grpc
from envoy.service.ext_proc.v3 import external_processor_pb2 as ext_proc
from envoy.type.v3 import http_status_pb2 as http_status

def decompress_body_if_needed(body_bytes: bytes, headers: dict) -> str:
    """
    Decodes body bytes. If the headers indicate the content is gzipped,
    it automatically decompresses it first.
    """
    if not body_bytes:
        return ""
    
    # Normalize headers to lowercase for robust lookup
    norm_headers = {k.lower(): v for k, v in headers.items()}
    is_gzip = norm_headers.get("content-encoding") == "gzip"
    
    if is_gzip:
        try:
            print("    [Gzip] Compressed payload detected, decompressing...", flush=True)
            decompressed = gzip.decompress(body_bytes)
            return decompressed.decode('utf-8', errors='replace')
        except Exception as e:
            print(f"    ⚠️ [Gzip] Failed to decompress gzip payload: {e}", flush=True)
            
    try:
        return body_bytes.decode('utf-8', errors='replace')
    except Exception as e:
        print(f"    ⚠️ Failed to decode payload bytes: {e}", flush=True)
        return ""

def is_req_valid(body_str: str, headers: dict) -> bool:
    """
    Validation hook for incoming requests.
    Returns True to allow the request, or False to block it immediately at the edge.
    """
    print("[Debugger Hook] Checking request validity...", flush=True)
    # Developers can implement custom validation rules here in the future
    return True

def is_resp_valid(body_str: str, headers: dict) -> bool:
    """
    Validation hook for outgoing backend responses.
    Returns True to allow the response, or False to block it before returning to the client.
    """
    print("[Debugger Hook] Checking response validity...", flush=True)
    # Developers can implement custom validation rules here in the future
    return True

class DebuggerProcessor(ext_proc_grpc.ExternalProcessorServicer):
    
    def Process(self, request_iterator, context):
        print("\n>>> 🟢 [Debugger v1.3.0-body-debug] New gRPC transaction stream opened!", flush=True)
        
        req_headers = {}
        resp_headers = {}
        
        for request in request_iterator:
            if request is None:
                print(">>> ⚠️ [Debugger] Received a completely NULL request object in the stream!", flush=True)
                continue
                
            field_name = request.WhichOneof("request")
            print(f"\n>>> 🔍 [Debugger] Received transaction phase: '{field_name}'", flush=True)
            
            # ==========================================
            # 1. REQUEST HEADERS PHASE
            # ==========================================
            if field_name == "request_headers":
                http_headers = request.request_headers
                if http_headers is None:
                    print(">>> 📋 [Debugger] request_headers field is NULL!", flush=True)
                else:
                    print(">>> 📋 [Debugger] ---------- REQUEST HEADERS ----------", flush=True)
                    headers_map = http_headers.headers
                    if headers_map is None:
                        print("    [Status] Headers Map object is NULL!", flush=True)
                    elif not headers_map.headers:
                        print("    [Status] Headers list is empty (non-null but empty).", flush=True)
                    else:
                        print(f"    [Status] Processing {len(headers_map.headers)} request headers...", flush=True)
                        for header in headers_map.headers:
                            if header is None:
                                print("    ⚠️ [Debugger] Header object is NULL!", flush=True)
                                continue
                            
                            # Log raw representation to see exact internal fields and values
                            print(f"    [Raw Header] {repr(header).strip()}", flush=True)
                            
                            # Access fields dynamically and robustly
                            key = header.key
                            val = ""
                            
                            # Dynamic resolution for raw_value (bytes/string) or value
                            if hasattr(header, "raw_value") and header.raw_value:
                                raw_val = header.raw_value
                                if isinstance(raw_val, bytes):
                                    val = raw_val.decode('utf-8', errors='replace')
                                else:
                                    val = str(raw_val)
                            elif hasattr(header, "value") and header.value:
                                raw_val = header.value
                                if isinstance(raw_val, bytes):
                                    val = raw_val.decode('utf-8', errors='replace')
                                else:
                                    val = str(raw_val)
                            
                            print(f"    [Parsed] {key}: {val}", flush=True)
                            req_headers[key.lower()] = val
                                
                    print(">>> 📋 [Debugger] -------------------------------------", flush=True)
                
                    # Forward request headers unaltered
                    yield ext_proc.ProcessingResponse(
                        request_headers=ext_proc.HeadersResponse(
                            response=ext_proc.CommonResponse()
                        )
                    )



            
            # ==========================================
            # 2. REQUEST BODY PHASE
            # ==========================================
            elif field_name == "request_body":
                http_body = request.request_body
                body_bytes = http_body.body if (http_body and http_body.body) else b""
                end_of_stream = getattr(http_body, "end_of_stream", False) if http_body else True
                
                # Log request body details
                print(">>> 📦 [Debugger] ---------- REQUEST BODY ----------", flush=True)
                print(f"    [Status] End of Stream: {end_of_stream}", flush=True)
                print(f"    [Status] Body size: {len(body_bytes)} bytes.", flush=True)
                body_str = ""
                if body_bytes:
                    body_str = decompress_body_if_needed(body_bytes, req_headers)
                    print(f"    Raw Body Content:\n{body_str}", flush=True)
                print(">>> 📦 [Debugger] ----------------------------------", flush=True)
                
                # Run the request validation hook
                if not is_req_valid(body_str, req_headers):
                    print(">>> 🚫 [Debugger] Request validation FAILED! Blocking request...", flush=True)
                    rejection = ext_proc.ImmediateResponse(
                        status=http_status.HttpStatus(code=http_status.StatusCode.Forbidden), # 403 Forbidden
                        body=json.dumps({
                            "status": "rejected", 
                            "reason": "Request blocked by edge debugger policy"
                        }).encode('utf-8'),
                        details="Rejected by request validation hook"
                    )
                    # Force response headers to application/json
                    header_opt = rejection.headers.set_headers.add()
                    header_opt.header.key = "content-type"
                    header_opt.header.value = "application/json"
                    
                    yield ext_proc.ProcessingResponse(immediate_response=rejection)
                    print(">>> 🚫 [Debugger] Rejection response sent. Closing stream.", flush=True)
                    return
                
                # Forward request body by returning the original body bytes in streamed_response.
                # In FULL_DUPLEX_STREAMED mode (required by CONTENT_AUTHZ), we must always return
                # streamed_response and propagate the end_of_stream flag, even if the body chunk is empty (0 bytes).
                resp = ext_proc.ProcessingResponse()
                resp.request_body.response.body_mutation.streamed_response.body = body_bytes
                resp.request_body.response.body_mutation.streamed_response.end_of_stream = end_of_stream
                yield resp

            # ==========================================
            # 3. RESPONSE HEADERS PHASE
            # ==========================================
            elif field_name == "response_headers":
                http_headers = request.response_headers
                if http_headers is None:
                    print(">>> 📋 [Debugger] response_headers field is NULL!", flush=True)
                else:
                    print(">>> 📋 [Debugger] ---------- RESPONSE HEADERS ----------", flush=True)
                    headers_map = http_headers.headers
                    if headers_map is None:
                        print("    [Status] Response Headers Map is NULL!", flush=True)
                    elif not headers_map.headers:
                        print("    [Status] Response headers list is empty.", flush=True)
                    else:
                        print(f"    [Status] Processing {len(headers_map.headers)} response headers...", flush=True)
                        for header in headers_map.headers:
                            if header is None:
                                print("    ⚠️ [Debugger] Header object is NULL!", flush=True)
                                continue
                            
                            # Log raw representation
                            print(f"    [Raw Header] {repr(header).strip()}", flush=True)
                            
                            # Access fields dynamically and robustly
                            key = header.key
                            val = ""
                            
                            # Dynamic resolution for raw_value (bytes/string) or value
                            if hasattr(header, "raw_value") and header.raw_value:
                                raw_val = header.raw_value
                                if isinstance(raw_val, bytes):
                                    val = raw_val.decode('utf-8', errors='replace')
                                else:
                                    val = str(raw_val)
                            elif hasattr(header, "value") and header.value:
                                raw_val = header.value
                                if isinstance(raw_val, bytes):
                                    val = raw_val.decode('utf-8', errors='replace')
                                else:
                                    val = str(raw_val)
                            
                            print(f"    [Parsed] {key}: {val}", flush=True)
                            resp_headers[key.lower()] = val
                                
                    print(">>> 📋 [Debugger] --------------------------------------", flush=True)
                
                # Forward response headers unaltered
                resp = ext_proc.ProcessingResponse()
                resp.response_headers.response.status = ext_proc.CommonResponse.ResponseStatus.CONTINUE
                yield resp
            
            # ==========================================
            # 4. RESPONSE BODY PHASE
            # ==========================================
            # ==========================================
            # 4. RESPONSE BODY PHASE
            # ==========================================
            elif field_name == "response_body":
                http_body = request.response_body
                body_bytes = http_body.body if (http_body and http_body.body) else b""
                end_of_stream = getattr(http_body, "end_of_stream", False) if http_body else True
                
                # Log response body details
                print(">>> 📦 [Debugger] ---------- RESPONSE BODY ----------", flush=True)
                print(f"    [Status] End of Stream: {end_of_stream}", flush=True)
                print(f"    [Status] Response body size: {len(body_bytes)} bytes.", flush=True)
                body_str = ""
                if body_bytes:
                    body_str = decompress_body_if_needed(body_bytes, resp_headers)
                    print(f"    Raw Response Body Content:\n{body_str}", flush=True)
                print(">>> 📦 [Debugger] -----------------------------------", flush=True)
                
                # Run the response validation hook
                if not is_resp_valid(body_str, resp_headers):
                    print(">>> 🚫 [Debugger] Response validation FAILED! Blocking response...", flush=True)
                    rejection = ext_proc.ImmediateResponse(
                        status=http_status.HttpStatus(code=http_status.StatusCode.Forbidden), # 403 Forbidden
                        body=json.dumps({
                            "status": "rejected", 
                            "reason": "Response blocked by edge debugger policy"
                        }).encode('utf-8'),
                        details="Rejected by response validation hook"
                    )
                    # Force response headers to application/json
                    header_opt = rejection.headers.set_headers.add()
                    header_opt.header.key = "content-type"
                    header_opt.header.value = "application/json"
                    
                    yield ext_proc.ProcessingResponse(immediate_response=rejection)
                    print(">>> 🚫 [Debugger] Rejection response sent. Closing stream.", flush=True)
                    return
                
                # Forward response body unaltered using streamed_response (required for FULL_DUPLEX_STREAMED mode).
                # We must always return streamed_response and propagate the end_of_stream flag to prevent
                # the response body from being stripped by Envoy, even if the body chunk is empty (0 bytes).
                resp = ext_proc.ProcessingResponse()
                print(f"    [Status] Copying original response body bytes ({len(body_bytes)} bytes) to streamed_response...", flush=True)
                resp.response_body.response.body_mutation.streamed_response.body = body_bytes
                resp.response_body.response.body_mutation.streamed_response.end_of_stream = end_of_stream
                yield resp

            
            else:
                print(f">>> ⚠️ [Debugger] Received unrecognized or unhandled request phase: '{field_name}'", flush=True)
        
        print(">>> 🔴 [Debugger] gRPC transaction stream closed cleanly.", flush=True)

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
    ext_proc_grpc.add_ExternalProcessorServicer_to_server(DebuggerProcessor(), server)
    
    cert_path = os.environ.get('SSL_CERT_PATH', '/app/fullchain.pem')
    key_path = os.environ.get('SSL_KEY_PATH', '/app/privkey.pem')
    
    if os.path.exists(cert_path) and os.path.exists(key_path):
        with open(key_path, 'rb') as f:
            private_key = f.read()
        with open(cert_path, 'rb') as f:
            certificate_chain = f.read()
        server_credentials = grpc.ssl_server_credentials(((private_key, certificate_chain),))
        server.add_secure_port('0.0.0.0:50051', server_credentials)
        print("Debugger Interceptor Server [v1.3.0-body-debug] running SECURELY with TLS on port 50051...", flush=True)
    else:
        server.add_insecure_port('0.0.0.0:50051')
        print("Debugger Interceptor Server [v1.3.0-body-debug] running in CLEAR TEXT on port 50051...", flush=True)
        
    server.start()
    server.wait_for_termination()


if __name__ == '__main__':
    serve()
