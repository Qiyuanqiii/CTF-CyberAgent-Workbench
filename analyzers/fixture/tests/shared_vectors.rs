use std::collections::HashSet;

use cyberagent_analyzer_fixture::{
    ALL_ERROR_CODES, ERROR_PROTOCOL_VERSION, EXIT_INTERNAL, EXIT_REJECTED, EXIT_SUCCESS,
    ErrorEnvelope, FIXTURE_ANALYZER_NAME, MAX_DECODED_INPUT_BYTES, MAX_MEDIA_TYPE_BYTES,
    MAX_REQUEST_ENVELOPE_BYTES, MAX_REQUEST_ID_BYTES, MAX_RESULT_ENVELOPE_BYTES,
    MAX_TIMEOUT_MILLISECONDS, MIN_RESULT_ENVELOPE_BYTES, MIN_TIMEOUT_MILLISECONDS,
    REQUEST_PROTOCOL_VERSION, RESULT_PROTOCOL_VERSION, ResultEnvelope, evaluate,
};
use serde::Deserialize;
use serde_json::Value;
use sha2::{Digest, Sha256};

const GOLDEN_PROTOCOL: &str = "analyzer_protocol_golden_vectors.v1";

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct GoldenFile {
    protocol_version: String,
    contract: GoldenContract,
    error_codes: Vec<String>,
    vectors: Vec<GoldenVector>,
}

#[derive(Debug, Deserialize, Eq, PartialEq)]
#[serde(deny_unknown_fields)]
struct GoldenContract {
    request_protocol: String,
    result_protocol: String,
    error_protocol: String,
    fixture_analyzer: String,
    max_request_envelope_bytes: usize,
    max_decoded_input_bytes: usize,
    min_result_envelope_bytes: usize,
    max_result_envelope_bytes: usize,
    min_timeout_ms: i64,
    max_timeout_ms: i64,
    max_request_id_bytes: usize,
    max_media_type_bytes: usize,
    exit_success: u8,
    exit_rejected: u8,
    exit_internal: u8,
}

#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct GoldenVector {
    name: String,
    request: Value,
    expected_exit_code: u8,
    expected_stdout: Value,
    expected_stdout_bytes: usize,
    expected_stdout_sha256: String,
}

#[test]
fn validates_shared_go_owned_golden_vectors() {
    let golden: GoldenFile = serde_json::from_str(include_str!(
        "../../testdata/analyzer_protocol_v1_vectors.json"
    ))
    .expect("shared golden vectors must parse strictly");
    assert_eq!(golden.protocol_version, GOLDEN_PROTOCOL);
    assert_eq!(
        golden.contract,
        GoldenContract {
            request_protocol: REQUEST_PROTOCOL_VERSION.to_owned(),
            result_protocol: RESULT_PROTOCOL_VERSION.to_owned(),
            error_protocol: ERROR_PROTOCOL_VERSION.to_owned(),
            fixture_analyzer: FIXTURE_ANALYZER_NAME.to_owned(),
            max_request_envelope_bytes: MAX_REQUEST_ENVELOPE_BYTES,
            max_decoded_input_bytes: MAX_DECODED_INPUT_BYTES,
            min_result_envelope_bytes: MIN_RESULT_ENVELOPE_BYTES,
            max_result_envelope_bytes: MAX_RESULT_ENVELOPE_BYTES,
            min_timeout_ms: MIN_TIMEOUT_MILLISECONDS,
            max_timeout_ms: MAX_TIMEOUT_MILLISECONDS,
            max_request_id_bytes: MAX_REQUEST_ID_BYTES,
            max_media_type_bytes: MAX_MEDIA_TYPE_BYTES,
            exit_success: EXIT_SUCCESS,
            exit_rejected: EXIT_REJECTED,
            exit_internal: EXIT_INTERNAL,
        }
    );
    let expected_codes = ALL_ERROR_CODES
        .iter()
        .map(|code| {
            serde_json::to_value(code)
                .unwrap()
                .as_str()
                .unwrap()
                .to_owned()
        })
        .collect::<Vec<_>>();
    assert_eq!(golden.error_codes, expected_codes);
    assert_eq!(golden.vectors.len(), 5);
    let mut seen = HashSet::new();
    for vector in golden.vectors {
        assert!(!vector.name.is_empty());
        assert!(
            seen.insert(vector.name.clone()),
            "duplicate {}",
            vector.name
        );
        let request = serde_json::to_vec(&vector.request).expect("request must serialize");
        let evaluation = evaluate(&request);
        let actual_sha = format!("{:x}", Sha256::digest(&evaluation.stdout));
        assert_eq!(
            evaluation.exit_code, vector.expected_exit_code,
            "{}",
            vector.name
        );
        assert_eq!(
            evaluation.stdout.len(),
            vector.expected_stdout_bytes,
            "{}",
            vector.name
        );
        assert_eq!(actual_sha, vector.expected_stdout_sha256, "{}", vector.name);
        let actual: Value = serde_json::from_slice(&evaluation.stdout).expect("output must parse");
        assert_eq!(actual, vector.expected_stdout, "{}", vector.name);
        if evaluation.exit_code == EXIT_SUCCESS {
            serde_json::from_slice::<ResultEnvelope>(&evaluation.stdout)
                .expect("success vector must use the strict result shape");
        } else {
            serde_json::from_slice::<ErrorEnvelope>(&evaluation.stdout)
                .expect("rejection vector must use the strict error shape");
        }
    }
}
