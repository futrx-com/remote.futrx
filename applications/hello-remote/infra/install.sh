#!/usr/bin/env bash
# Idempotent application-specific provisioning. Remote owns the Go build,
# systemd unit, environment, health check, and lifecycle declared elsewhere.
set -euo pipefail

readonly service_user="hello-remote"
readonly service_group="hello-remote"
readonly state_dir="/var/lib/hello-remote"
readonly version_file="${state_dir}/provisioned-version"
readonly application_version="${APP_APPLICATION_VERSION:?Remote did not provide APP_APPLICATION_VERSION}"

if ! getent group "${service_group}" >/dev/null; then
  groupadd --system "${service_group}"
fi

if ! id -u "${service_user}" >/dev/null 2>&1; then
  useradd \
    --system \
    --gid "${service_group}" \
    --home-dir "${state_dir}" \
    --shell /usr/sbin/nologin \
    "${service_user}"
fi

install -d -o "${service_user}" -g "${service_group}" -m 0750 "${state_dir}"

# Replace application-owned metadata atomically so retries and upgrades
# converge without resetting any other state the service may keep here.
temporary_file="$(mktemp "${state_dir}/.provisioned-version.XXXXXX")"
trap 'rm -f -- "${temporary_file}"' EXIT
printf '%s\n' "${application_version}" >"${temporary_file}"
chown "${service_user}:${service_group}" "${temporary_file}"
chmod 0640 "${temporary_file}"
mv -f -- "${temporary_file}" "${version_file}"
trap - EXIT

echo "install: provisioned ${APP_APPLICATION_NAME} ${application_version} for ${APP_SERVICE}"
