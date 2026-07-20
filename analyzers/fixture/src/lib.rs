use std::io::{self, Read, Write};

use base64::{Engine as _, engine::general_purpose::STANDARD};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};

pub const REQUEST_PROTOCOL_VERSION: &str = "analyzer_protocol.v1";
pub const RESULT_PROTOCOL_VERSION: &str = "analyzer_result.v1";
pub const ERROR_PROTOCOL_VERSION: &str = "analyzer_error.v1";
pub const FIXTURE_ANALYZER_NAME: &str = "fixture.digest.v1";

pub const MAX_REQUEST_ENVELOPE_BYTES: usize = 96 * 1024;
pub const MAX_DECODED_INPUT_BYTES: usize = 64 * 1024;
pub const MIN_RESULT_ENVELOPE_BYTES: usize = 512;
pub const MAX_RESULT_ENVELOPE_BYTES: usize = 16 * 1024;
pub const MIN_TIMEOUT_MILLISECONDS: i64 = 100;
pub const MAX_TIMEOUT_MILLISECONDS: i64 = 30_000;
pub const MAX_REQUEST_ID_BYTES: usize = 128;
pub const MAX_MEDIA_TYPE_BYTES: usize = 128;

pub const EXIT_SUCCESS: u8 = 0;
pub const EXIT_REJECTED: u8 = 2;
pub const EXIT_INTERNAL: u8 = 3;

#[derive(Clone, Copy, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum ErrorCode {
    MalformedEnvelope,
    RequestTooLarge,
    UnsupportedProtocol,
    InvalidRequest,
    CapabilityDenied,
    UnsupportedAnalyzer,
    InputLimitExceeded,
    InvalidContent,
    OutputLimitExceeded,
    ResultTooLarge,
    DeadlineExceeded,
    ProcessFailed,
    InvalidResult,
    InternalError,
}

pub const ALL_ERROR_CODES: [ErrorCode; 14] = [
    ErrorCode::MalformedEnvelope,
    ErrorCode::RequestTooLarge,
    ErrorCode::UnsupportedProtocol,
    ErrorCode::InvalidRequest,
    ErrorCode::CapabilityDenied,
    ErrorCode::UnsupportedAnalyzer,
    ErrorCode::InputLimitExceeded,
    ErrorCode::InvalidContent,
    ErrorCode::OutputLimitExceeded,
    ErrorCode::ResultTooLarge,
    ErrorCode::DeadlineExceeded,
    ErrorCode::ProcessFailed,
    ErrorCode::InvalidResult,
    ErrorCode::InternalError,
];

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Capabilities {
    pub filesystem: bool,
    pub network: bool,
    pub subprocess: bool,
    pub environment: bool,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Limits {
    pub max_input_bytes: i64,
    pub max_output_bytes: i64,
    pub timeout_ms: i64,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Input {
    pub media_type: String,
    pub content_base64: String,
}

#[derive(Clone, Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Request {
    pub protocol_version: String,
    pub request_id: String,
    pub analyzer: String,
    pub input: Input,
    pub limits: Limits,
    pub capabilities: Capabilities,
    pub metadata_only: bool,
}

#[derive(Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct Summary {
    pub media_type: String,
    pub input_bytes: usize,
    pub sha256: String,
    pub utf8: bool,
    pub line_count: usize,
}

#[derive(Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ResultEnvelope {
    pub protocol_version: String,
    pub request_id: String,
    pub analyzer: String,
    pub status: String,
    pub summary: Summary,
    pub metadata_only: bool,
    pub capabilities_used: Capabilities,
}

#[derive(Debug, Deserialize, Eq, PartialEq, Serialize)]
#[serde(deny_unknown_fields)]
pub struct ErrorEnvelope {
    pub protocol_version: String,
    pub request_id: String,
    pub code: ErrorCode,
    pub retryable: bool,
    pub message: String,
    pub metadata_only: bool,
}

#[derive(Debug, Eq, PartialEq)]
pub struct Evaluation {
    pub stdout: Vec<u8>,
    pub exit_code: u8,
}

pub fn evaluate(raw: &[u8]) -> Evaluation {
    if raw.len() > MAX_REQUEST_ENVELOPE_BYTES {
        return rejection("", ErrorCode::RequestTooLarge);
    }
    if raw.is_empty() {
        return rejection("", ErrorCode::MalformedEnvelope);
    }
    let wire: RequestWire = match serde_json::from_slice(raw) {
        Ok(value) => value,
        Err(_) => return rejection("", ErrorCode::MalformedEnvelope),
    };
    let request = match wire.into_request() {
        Ok(value) => value,
        Err(code) => return rejection("", code),
    };
    if let Some(code) = validate_request(&request) {
        return rejection(safe_request_id(&request.request_id), code);
    }
    let content = match decode_content(&request) {
        Ok(value) => value,
        Err(code) => return rejection(&request.request_id, code),
    };
    let text = std::str::from_utf8(&content).is_ok();
    let result = ResultEnvelope {
        protocol_version: RESULT_PROTOCOL_VERSION.to_owned(),
        request_id: request.request_id.clone(),
        analyzer: request.analyzer,
        status: "succeeded".to_owned(),
        summary: Summary {
            media_type: request.input.media_type,
            input_bytes: content.len(),
            sha256: format!("{:x}", Sha256::digest(&content)),
            utf8: text,
            line_count: logical_line_count(&content, text),
        },
        metadata_only: true,
        capabilities_used: disabled_capabilities(),
    };
    let encoded = match serde_json::to_vec(&result) {
        Ok(value) => value,
        Err(_) => return rejection(&request.request_id, ErrorCode::InternalError),
    };
    if encoded.len() > request.limits.max_output_bytes as usize
        || encoded.len() > MAX_RESULT_ENVELOPE_BYTES
    {
        return rejection(&request.request_id, ErrorCode::OutputLimitExceeded);
    }
    Evaluation {
        stdout: encoded,
        exit_code: EXIT_SUCCESS,
    }
}

pub fn run_io<R: Read, W: Write>(reader: R, mut writer: W) -> io::Result<u8> {
    let mut raw = Vec::with_capacity(4096);
    reader
        .take((MAX_REQUEST_ENVELOPE_BYTES + 1) as u64)
        .read_to_end(&mut raw)?;
    let evaluation = evaluate(&raw);
    writer.write_all(&evaluation.stdout)?;
    writer.flush()?;
    Ok(evaluation.exit_code)
}

pub fn internal_error() -> Vec<u8> {
    encode_error("", ErrorCode::InternalError)
}

fn validate_request(request: &Request) -> Option<ErrorCode> {
    if request.protocol_version != REQUEST_PROTOCOL_VERSION {
        return Some(ErrorCode::UnsupportedProtocol);
    }
    if !valid_request_id(&request.request_id) {
        return Some(ErrorCode::InvalidRequest);
    }
    if request.analyzer != FIXTURE_ANALYZER_NAME {
        return Some(ErrorCode::UnsupportedAnalyzer);
    }
    if capabilities_enabled(&request.capabilities) {
        return Some(ErrorCode::CapabilityDenied);
    }
    if !request.metadata_only
        || request.limits.max_input_bytes < 1
        || request.limits.max_input_bytes > MAX_DECODED_INPUT_BYTES as i64
        || request.limits.max_output_bytes < MIN_RESULT_ENVELOPE_BYTES as i64
        || request.limits.max_output_bytes > MAX_RESULT_ENVELOPE_BYTES as i64
        || request.limits.timeout_ms < MIN_TIMEOUT_MILLISECONDS
        || request.limits.timeout_ms > MAX_TIMEOUT_MILLISECONDS
        || !valid_media_type(&request.input.media_type)
    {
        return Some(ErrorCode::InvalidRequest);
    }
    decode_content(request).err()
}

fn decode_content(request: &Request) -> Result<Vec<u8>, ErrorCode> {
    let maximum_encoded = MAX_DECODED_INPUT_BYTES.div_ceil(3) * 4;
    if request.input.content_base64.len() > maximum_encoded {
        return Err(ErrorCode::InputLimitExceeded);
    }
    let content = STANDARD
        .decode(&request.input.content_base64)
        .map_err(|_| ErrorCode::InvalidContent)?;
    if STANDARD.encode(&content) != request.input.content_base64 {
        return Err(ErrorCode::InvalidContent);
    }
    if content.len() > request.limits.max_input_bytes as usize
        || content.len() > MAX_DECODED_INPUT_BYTES
    {
        return Err(ErrorCode::InputLimitExceeded);
    }
    Ok(content)
}

fn rejection(request_id: &str, code: ErrorCode) -> Evaluation {
    Evaluation {
        stdout: encode_error(request_id, code),
        exit_code: EXIT_REJECTED,
    }
}

fn encode_error(request_id: &str, code: ErrorCode) -> Vec<u8> {
    serde_json::to_vec(&ErrorEnvelope {
        protocol_version: ERROR_PROTOCOL_VERSION.to_owned(),
        request_id: request_id.to_owned(),
        code,
        retryable: retryable(code),
        message: message_for(code).to_owned(),
        metadata_only: true,
    })
    .expect("fixed analyzer error envelope must serialize")
}

fn logical_line_count(content: &[u8], text: bool) -> usize {
    if !text || content.is_empty() {
        return 0;
    }
    let mut count = content.iter().filter(|byte| **byte == b'\n').count();
    if content.last() != Some(&b'\n') {
        count += 1;
    }
    count
}

fn disabled_capabilities() -> Capabilities {
    Capabilities {
        filesystem: false,
        network: false,
        subprocess: false,
        environment: false,
    }
}

fn capabilities_enabled(value: &Capabilities) -> bool {
    value.filesystem || value.network || value.subprocess || value.environment
}

fn valid_request_id(value: &str) -> bool {
    !value.is_empty()
        && value.len() <= MAX_REQUEST_ID_BYTES
        && value
            .bytes()
            .all(|byte| byte.is_ascii_alphanumeric() || matches!(byte, b'.' | b'_' | b':' | b'-'))
}

fn safe_request_id(value: &str) -> &str {
    if valid_request_id(value) { value } else { "" }
}

fn valid_media_type(value: &str) -> bool {
    if value.len() < 3 || value.len() > MAX_MEDIA_TYPE_BYTES || value.matches('/').count() != 1 {
        return false;
    }
    value.split('/').all(|part| {
        !part.is_empty()
            && part.bytes().all(|byte| {
                byte.is_ascii_lowercase()
                    || byte.is_ascii_digit()
                    || matches!(
                        byte,
                        b'!' | b'#' | b'$' | b'&' | b'^' | b'_' | b'.' | b'+' | b'-'
                    )
            })
    })
}

fn message_for(code: ErrorCode) -> &'static str {
    match code {
        ErrorCode::MalformedEnvelope => "analyzer request is malformed",
        ErrorCode::RequestTooLarge => "analyzer request exceeds the envelope limit",
        ErrorCode::UnsupportedProtocol => "analyzer protocol is unsupported",
        ErrorCode::InvalidRequest => "analyzer request violates the protocol contract",
        ErrorCode::CapabilityDenied => "requested analyzer capability is disabled",
        ErrorCode::UnsupportedAnalyzer => "analyzer is unsupported",
        ErrorCode::InputLimitExceeded => "analyzer input exceeds its declared limit",
        ErrorCode::InvalidContent => "analyzer input content is invalid",
        ErrorCode::OutputLimitExceeded => "analyzer output exceeds its declared limit",
        ErrorCode::ResultTooLarge => "analyzer result exceeds the envelope limit",
        ErrorCode::DeadlineExceeded => "analyzer deadline was exceeded",
        ErrorCode::ProcessFailed => "analyzer process failed",
        ErrorCode::InvalidResult => "analyzer result violates the protocol contract",
        ErrorCode::InternalError => "analyzer failed internally",
    }
}

fn retryable(code: ErrorCode) -> bool {
    matches!(code, ErrorCode::DeadlineExceeded | ErrorCode::ProcessFailed)
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct RequestWire {
    protocol_version: Option<String>,
    request_id: Option<String>,
    analyzer: Option<String>,
    input: Option<InputWire>,
    limits: Option<LimitsWire>,
    capabilities: Option<CapabilitiesWire>,
    metadata_only: Option<bool>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct InputWire {
    media_type: Option<String>,
    content_base64: Option<String>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct LimitsWire {
    max_input_bytes: Option<i64>,
    max_output_bytes: Option<i64>,
    timeout_ms: Option<i64>,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct CapabilitiesWire {
    filesystem: Option<bool>,
    network: Option<bool>,
    subprocess: Option<bool>,
    environment: Option<bool>,
}

impl RequestWire {
    fn into_request(self) -> Result<Request, ErrorCode> {
        let input = self.input.ok_or(ErrorCode::InvalidRequest)?;
        let limits = self.limits.ok_or(ErrorCode::InvalidRequest)?;
        let capabilities = self.capabilities.ok_or(ErrorCode::InvalidRequest)?;
        Ok(Request {
            protocol_version: self.protocol_version.ok_or(ErrorCode::InvalidRequest)?,
            request_id: self.request_id.ok_or(ErrorCode::InvalidRequest)?,
            analyzer: self.analyzer.ok_or(ErrorCode::InvalidRequest)?,
            input: Input {
                media_type: input.media_type.ok_or(ErrorCode::InvalidRequest)?,
                content_base64: input.content_base64.ok_or(ErrorCode::InvalidRequest)?,
            },
            limits: Limits {
                max_input_bytes: limits.max_input_bytes.ok_or(ErrorCode::InvalidRequest)?,
                max_output_bytes: limits.max_output_bytes.ok_or(ErrorCode::InvalidRequest)?,
                timeout_ms: limits.timeout_ms.ok_or(ErrorCode::InvalidRequest)?,
            },
            capabilities: Capabilities {
                filesystem: capabilities.filesystem.ok_or(ErrorCode::InvalidRequest)?,
                network: capabilities.network.ok_or(ErrorCode::InvalidRequest)?,
                subprocess: capabilities.subprocess.ok_or(ErrorCode::InvalidRequest)?,
                environment: capabilities.environment.ok_or(ErrorCode::InvalidRequest)?,
            },
            metadata_only: self.metadata_only.ok_or(ErrorCode::InvalidRequest)?,
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn request() -> Request {
        Request {
            protocol_version: REQUEST_PROTOCOL_VERSION.to_owned(),
            request_id: "request-1".to_owned(),
            analyzer: FIXTURE_ANALYZER_NAME.to_owned(),
            input: Input {
                media_type: "application/octet-stream".to_owned(),
                content_base64: String::new(),
            },
            limits: Limits {
                max_input_bytes: MAX_DECODED_INPUT_BYTES as i64,
                max_output_bytes: 4096,
                timeout_ms: 5000,
            },
            capabilities: disabled_capabilities(),
            metadata_only: true,
        }
    }

    #[test]
    fn evaluates_metadata_without_returning_content() {
        let mut value = request();
        value.input.media_type = "text/plain".to_owned();
        value.input.content_base64 = STANDARD.encode(b"alpha\nbeta\n");
        let evaluation = evaluate(&serde_json::to_vec(&value).unwrap());
        assert_eq!(evaluation.exit_code, EXIT_SUCCESS);
        let result: ResultEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(result.summary.input_bytes, 11);
        assert_eq!(result.summary.line_count, 2);
        assert!(result.summary.utf8);
        assert_eq!(
            result.summary.sha256,
            "e49c81e2d2f84e259d40e2fb8192f3bcd198b355184845d76d8f58807d0d78ee"
        );
        assert!(!evaluation.stdout.windows(5).any(|part| part == b"alpha"));
        assert!(!capabilities_enabled(&result.capabilities_used));
    }

    #[test]
    fn rejects_unknown_duplicate_and_enabled_capability() {
        let valid = String::from_utf8(serde_json::to_vec(&request()).unwrap()).unwrap();
        for malformed in [
            valid.replace(
                "\"metadata_only\":true",
                "\"metadata_only\":true,\"extra\":false",
            ),
            valid.replace("\"network\":false", "\"network\":false,\"network\":false"),
        ] {
            let evaluation = evaluate(malformed.as_bytes());
            let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
            assert_eq!(error.code, ErrorCode::MalformedEnvelope);
            assert!(error.request_id.is_empty());
        }
        let deep = format!("{}{}", "[".repeat(256), "]".repeat(256));
        let evaluation = evaluate(deep.as_bytes());
        let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(error.code, ErrorCode::MalformedEnvelope);
        assert!(error.request_id.is_empty());

        let enabled = valid.replace("\"network\":false", "\"network\":true");
        let evaluation = evaluate(enabled.as_bytes());
        let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(error.code, ErrorCode::CapabilityDenied);
        assert_eq!(error.request_id, "request-1");

        let missing = valid.replace(",\"environment\":false", "");
        let evaluation = evaluate(missing.as_bytes());
        let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(error.code, ErrorCode::InvalidRequest);
        assert!(error.request_id.is_empty());
    }

    #[test]
    fn enforces_declared_input_and_output_limits() {
        let mut input_limited = request();
        input_limited.limits.max_input_bytes = 4;
        input_limited.input.content_base64 = STANDARD.encode(b"12345");
        let evaluation = evaluate(&serde_json::to_vec(&input_limited).unwrap());
        let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(error.code, ErrorCode::InputLimitExceeded);

        let mut output_limited = request();
        output_limited.request_id = "r".repeat(MAX_REQUEST_ID_BYTES);
        output_limited.input.media_type = format!("application/{}", "x".repeat(116));
        output_limited.limits.max_output_bytes = MIN_RESULT_ENVELOPE_BYTES as i64;
        let evaluation = evaluate(&serde_json::to_vec(&output_limited).unwrap());
        let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(error.code, ErrorCode::OutputLimitExceeded);
    }

    #[test]
    fn bounds_stdin_before_parsing() {
        let raw = vec![b' '; MAX_REQUEST_ENVELOPE_BYTES + 20];
        let mut stdout = Vec::new();
        let exit = run_io(raw.as_slice(), &mut stdout).unwrap();
        let error: ErrorEnvelope = serde_json::from_slice(&stdout).unwrap();
        assert_eq!(exit, EXIT_REJECTED);
        assert_eq!(error.code, ErrorCode::RequestTooLarge);
    }

    #[test]
    fn does_not_reflect_an_invalid_request_id() {
        let mut value = request();
        value.request_id = "private value".to_owned();
        let evaluation = evaluate(&serde_json::to_vec(&value).unwrap());
        let error: ErrorEnvelope = serde_json::from_slice(&evaluation.stdout).unwrap();
        assert_eq!(error.code, ErrorCode::InvalidRequest);
        assert!(error.request_id.is_empty());
    }
}
