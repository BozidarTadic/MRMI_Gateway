package version

// App is the gateway software version. Bump when cutting a release.
const App = "0.1.0"

// ADR is the Architecture Decision Record version that this build
// fully implements. Keep in sync with docs/MRMI_Gateway_ADR_v<ADR>.md.
// When all sprint tasks for a given ADR version are complete, App and
// ADR minor versions should align (e.g. ADR 0.8 → App 0.8.0).
const ADR = "0.8"
