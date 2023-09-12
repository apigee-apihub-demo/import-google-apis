# Copyright 2023 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.20.1 as builder
WORKDIR /app
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -v -o import-google-apis .

FROM ghcr.io/apigee-apihub-demo/registry-kit:latest
COPY --from=builder /app/import-google-apis /usr/local/bin/import-google-apis
COPY RUN.sh /RUN.sh
CMD ["/RUN.sh"]
