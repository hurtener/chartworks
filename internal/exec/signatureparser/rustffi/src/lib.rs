//! Structural evidence companion for the pinned warehouse parser. This does not
//! decide statement safety, dependency access, signatures or source validity.
use polyglot_sql::traversal::ExpressionWalk;
use polyglot_sql::{parse, DialectType};
use std::ffi::{c_char, CStr, CString};
use std::str::FromStr;

fn inspect(query: &str, dialect: &str, nodes: usize, depth: usize) -> Result<String, ()> {
    if query.len() > 262144 || dialect.len() > 32 {
        return Err(());
    }
    let query = query.to_owned();
    let dialect = dialect.to_owned();
    // The parser uses substantial recursive frames even for small trees. A
    // fixed-stack joined worker avoids depending on the caller thread stack.
    std::thread::Builder::new()
        .stack_size(if cfg!(debug_assertions) {
            64 * 1024 * 1024
        } else {
            16 * 1024 * 1024
        })
        .spawn(move || inspect_owned(&query, &dialect, nodes, depth))
        .map_err(|_| ())?
        .join()
        .map_err(|_| ())?
}

fn inspect_owned(query: &str, dialect: &str, nodes: usize, depth: usize) -> Result<String, ()> {
    if query.is_empty()
        || query.len() > 262144
        || nodes == 0
        || nodes > 100000
        || depth == 0
        || depth > 64
    {
        return Err(());
    }
    // Same conservative pre-parser work bound as the primary parser. This is
    // intentionally lexical admission, not expression or safety interpretation.
    if query.chars().filter(|c| !c.is_whitespace()).count() > 512 {
        return Err(());
    }
    let mut nesting = 0i32;
    for c in query.chars() {
        match c {
            '(' | '[' => {
                nesting += 1;
                if nesting as usize > depth {
                    return Err(());
                }
            }
            ')' | ']' => {
                nesting -= 1;
                if nesting < 0 {
                    return Err(());
                }
            }
            _ => (),
        }
    }
    if nesting != 0 {
        return Err(());
    }
    if !matches!(
        dialect,
        "mysql" | "tsql" | "bigquery" | "snowflake" | "databricks"
    ) {
        return Err(());
    }
    let parsed = parse(query, DialectType::from_str(dialect).map_err(|_| ())?).map_err(|_| ())?;
    if parsed.len() != 1 {
        return Err(());
    }
    let root = &parsed[0];
    if root.dfs().take(nodes + 1).count() > nodes || root.tree_depth() > depth {
        return Err(());
    }
    let result = serde_json::to_string(root).map_err(|_| ())?;
    if result.len() > 4194304 {
        return Err(());
    }
    Ok(result)
}

// Bind the companion archive to the exact checked-in parser contract. The Go
// consumer compares this static text with its embedded source before parsing.
// This also makes stale native-cache restoration fail closed.
#[no_mangle]
pub extern "C" fn chartworks_signature_contract() -> *const c_char {
    concat!(
        include_str!("../Cargo.toml"),
        include_str!("../Cargo.lock"),
        include_str!("lib.rs"),
        "\0"
    )
    .as_ptr() as *const c_char
}

/// Input pointers are owned by the calling Go wrapper for the entire call.
#[no_mangle]
pub unsafe extern "C" fn chartworks_signature_ast(
    query: *const c_char,
    dialect: *const c_char,
    nodes: usize,
    depth: usize,
) -> *mut c_char {
    let result = std::panic::catch_unwind(|| {
        if query.is_null() || dialect.is_null() {
            return Err(());
        }
        let query = CStr::from_ptr(query).to_str().map_err(|_| ())?;
        let dialect = CStr::from_ptr(dialect).to_str().map_err(|_| ())?;
        inspect(query, dialect, nodes, depth)
    });
    let payload = match result {
        Ok(Ok(value)) => value,
        _ => "{\"error\":true}".to_owned(),
    };
    CString::new(payload)
        .unwrap_or_else(|_| CString::new("{\"error\":true}").unwrap())
        .into_raw()
}

#[no_mangle]
pub unsafe extern "C" fn chartworks_signature_free(value: *mut c_char) {
    if !value.is_null() {
        drop(CString::from_raw(value));
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    #[test]
    fn structural_ast_and_bounds() {
        for dialect in ["mysql", "tsql", "bigquery", "snowflake", "databricks"] {
            let ast = inspect(
                "SELECT round(sum(amount), 2) AS total FROM analytics.sales",
                dialect,
                1000,
                32,
            )
            .unwrap();
            assert!(ast.contains("round") && ast.contains("sum"));
        }
        let nested = format!("SELECT {}1{}", "(".repeat(60), ")".repeat(60));
        assert!(inspect(&nested, "mysql", 1000, 64).is_ok());
        let cases = format!(
            "SELECT {}1{}",
            "CASE WHEN TRUE THEN ".repeat(18),
            " ELSE 0 END".repeat(18)
        );
        assert!(inspect(&cases, "mysql", 1000, 64).is_ok());
        let unary = format!("SELECT {}1", "- ".repeat(490));
        assert!(inspect(&unary, "mysql", 1000, 64).is_err());
        assert!(inspect("SELECT 1; SELECT 2", "mysql", 1000, 32).is_err());
        assert!(inspect("SELECT abs(1)", "mysql", 1, 32).is_err());
        assert!(inspect("SELECT abs(1)", "unknown", 1000, 32).is_err());
        assert!(inspect(
            &format!("SELECT {}1{}", "(".repeat(1000), ")".repeat(1000)),
            "mysql",
            1000,
            32
        )
        .is_err());
    }
}
