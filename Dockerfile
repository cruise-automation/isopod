# Copyright 2019 GM Cruise LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http:#www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

FROM golang:1.14.0-alpine3.11 as builder

WORKDIR /build
COPY . /build

RUN CGO_ENABLED=0 GOOS=linux GO111MODULE=on go build -mod=vendor


FROM gcr.io/distroless/base

WORKDIR /cruise/paas/bin
COPY --from=builder /build/isopod .

ENTRYPOINT ["./isopod"]
