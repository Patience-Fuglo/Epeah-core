use sha2::{Sha256, Digest};
use std::fs::OpenOptions;
use std::io::Write;
use chrono::Utc;

pub struct ComplianceBlock {
    pub timestamp: i64,
    pub tx_hash: [u8; 32],
    pub data_payload: String,
}

pub struct AuditLedger {
    storage_path: String,
    current_root: [u8; 32],
}

impl AuditLedger {
    pub fn new(path: &str) -> Self {
        Self {
            storage_path: path.to_string(),
            current_root: [0u8; 32],
        }
    }

    pub fn commit_entry(
        &mut self,
        agent_id: &str,
        ticker: &str,
        status: &str,
        raw_payload: &str,
    ) -> Result<[u8; 32], std::io::Error> {
        let timestamp = Utc::now().timestamp_nanos_opt().unwrap_or(0);

        let target_data = format!("{}:{}:{}:{}:{}", timestamp, agent_id, ticker, status, raw_payload);

        let mut hasher = Sha256::new();
        hasher.update(self.current_root);
        hasher.update(target_data.as_bytes());
        let new_hash: [u8; 32] = hasher.finalize().into();

        self.current_root = new_hash;

        let mut file = OpenOptions::new()
            .create(true)
            .append(true)
            .open(&self.storage_path)?;

        let log_line = format!(
            "{{\"timestamp\":{},\"hash\":\"{}\",\"data\":\"{}\"}}\n",
            timestamp,
            hash_to_string(&new_hash),
            target_data.replace("\"", "\\\"")
        );

        file.write_all(log_line.as_bytes())?;

        Ok(new_hash)
    }
}

fn hash_to_string(bytes: &[u8; 32]) -> String {
    bytes.iter().map(|b| format!("{:02x}", b)).collect()
}
