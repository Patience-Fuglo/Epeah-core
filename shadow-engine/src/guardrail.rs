use regex::RegexSet;
use lazy_static::lazy_static;

lazy_static! {
    static ref SEMANTIC_BLACKLIST: RegexSet = RegexSet::new(&[
        r"(?i)ignore previous instructions",
        r"(?i)system override",
        r"(?i)as an ai model, i cannot",
        r"(?i)infinite loop",
        r"(?i)execute without hedge",
        r"(?i)disregard max position size",
    ]).unwrap();
}

pub struct ContextGuardrail;

impl ContextGuardrail {
    pub fn is_hallucinating_or_compromised(reasoning_tokens: &str) -> bool {
        if SEMANTIC_BLACKLIST.is_match(reasoning_tokens) {
            let matches: Vec<_> = SEMANTIC_BLACKLIST.matches(reasoning_tokens).into_iter().collect();
            println!(
                "[GUARDRAIL TRIGGERED] Semantic violation caught in reasoning tokens. Pattern matches: {:?}",
                matches
            );
            return true;
        }
        false
    }
}
