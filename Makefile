generate:
	protoc -I api/proto --go_out=api/go --go_opt=paths=source_relative api/proto/nginx/v1/nginx_config.proto
	protoc -I api/proto --go-grpc_out=api/go --go-grpc_opt=paths=source_relative api/proto/nginx/v1/nginx_config.proto
