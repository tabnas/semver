// The translation parts (admin ADR-27): what the manifest says and what
// the crate embeds are the same files.
//
// A packaged crate holds nothing outside `rs/`, so the crate embeds its
// own copies, `rs/translate/manifest.json` of `tabnas.plugin.json`,
// `rs/translate/render.alc` of the render the manifest names and
// `rs/translate/embed.alc` of the embedding it names, as
// `manifest_text()`, `render_text()` and `translate().embed`. The
// copies are the only texts a host sees, so they must be the files: this
// holds the embedded manifest to the repository's, and the render and the
// embedding the manifest names, read from the repository, to the embedded
// ones. Change the file at the root and run `npm run embed` (from `ts/`),
// which copies it into `rs/translate/`; this fails until both are the
// same.

use std::fs;
use std::path::PathBuf;

use serde_json::Value;

fn repo_root() -> PathBuf {
    let rs = PathBuf::from(env!("CARGO_MANIFEST_DIR"));
    rs.parent().expect("rs/ has a parent").to_path_buf()
}

fn translate() -> Value {
    let manifest: Value =
        serde_json::from_str(tabnas_semver::manifest_text()).expect("the manifest is JSON");
    manifest
        .get("translate")
        .cloned()
        .expect("the manifest carries a translate object")
}

#[test]
fn the_manifest_the_crate_embeds_is_the_repositorys() {
    let on_disk = fs::read_to_string(repo_root().join("tabnas.plugin.json"))
        .expect("the repository has its manifest");
    assert_eq!(
        on_disk,
        tabnas_semver::manifest_text(),
        "rs/translate/manifest.json is not tabnas.plugin.json: run npm run embed"
    );
}

#[test]
fn the_render_the_manifest_names_is_the_one_the_crate_embeds() {
    let translate = translate();
    let path = translate["render"]
        .as_str()
        .expect("translate.render names a file");
    let on_disk = fs::read_to_string(repo_root().join(path))
        .unwrap_or_else(|e| panic!("translate.render names {path}, which cannot be read: {e}"));
    assert_eq!(
        on_disk,
        tabnas_semver::render_text(),
        "translate.render names {path}, and rs/translate/render.alc, which render_text() \
         embeds, is another text: run npm run embed"
    );
}

#[test]
fn the_embed_the_manifest_names_is_the_one_the_crate_embeds() {
    let translate = translate();
    let path = translate["embed"]
        .as_str()
        .expect("translate.embed names a file");
    let on_disk = fs::read_to_string(repo_root().join(path))
        .unwrap_or_else(|e| panic!("translate.embed names {path}, which cannot be read: {e}"));
    let parts = tabnas_semver::translate().expect("Semantic Versioning carries translation parts");
    let embed = parts
        .embed
        .expect("Semantic Versioning carries an embedding");
    assert_eq!(
        Some(on_disk.as_str()),
        embed.source,
        "translate.embed names {path}, and rs/translate/embed.alc, which the crate \
         embeds, is another text: run npm run embed"
    );
}

#[test]
fn the_structural_interface_names_the_render_and_embed_entries() {
    let parts = tabnas_semver::translate().expect("Semantic Versioning carries translation parts");
    assert_eq!(parts.manifest, tabnas_semver::manifest_text());
    assert_eq!(parts.lift, None);
    let render = parts.render.expect("Semantic Versioning carries a render");
    assert_eq!(render.entry, "semver-render");
    assert_eq!(render.source, Some(tabnas_semver::render_text()));
    let embed = parts
        .embed
        .expect("Semantic Versioning carries an embedding");
    assert_eq!(embed.entry, "semver-embed");
    assert!(
        embed.source.is_some(),
        "the embedding is the format's own file"
    );
}

/// Semantic Versioning is read as a tree and written from one, whose events carry
/// it already, so there is no lift. That tree has a schema of its own,
/// `semver`, which a plain tree reaches through the embedding, and the
/// render takes an object at the root.
#[test]
fn semver_reads_and_writes_a_tree_of_its_schema() {
    let manifest: Value =
        serde_json::from_str(tabnas_semver::manifest_text()).expect("the manifest is JSON");
    assert_eq!(manifest["languageId"], "semver");
    let translate = translate();
    assert_eq!(translate["reads"], "tree");
    assert_eq!(translate["writes"], "tree");
    assert_eq!(translate["root"], "object");
    assert_eq!(translate["schema"], "semver");
    assert_eq!(translate["embed"], "alchemy/embed.alc");
    assert_eq!(translate["render"], "alchemy/render.alc");
    assert_eq!(translate.get("lift"), None);
}

/// The host prints the loss lines verbatim, so each is a sentence.
#[test]
fn the_loss_is_a_list_of_sentences() {
    let translate = translate();
    let loss = translate["loss"]
        .as_array()
        .expect("translate.loss is a list");
    assert!(!loss.is_empty());
    for line in loss {
        let line = line.as_str().expect("each loss line is a string");
        assert!(
            line.starts_with(char::is_uppercase) && line.ends_with('.'),
            "{line:?} is not a sentence"
        );
    }
}

/// The names a library of alchemy definitions defines, in its order.
fn definitions(text: &str) -> Vec<&str> {
    text.lines()
        .filter_map(|line| line.strip_prefix("def "))
        .filter_map(|rest| rest.split_whitespace().next())
        .collect()
}

/// A host links the render and the embedding with its own program and
/// other formats' parts into one namespace, so every definition of both
/// is named for Semantic Versioning, both entry points of the embedding
/// are there, and none of its names is one of the render's.
#[test]
fn the_parts_are_libraries_named_for_semver() {
    let render = definitions(tabnas_semver::render_text());
    assert!(render.contains(&"semver-render"), "{render:?}");
    let parts = tabnas_semver::translate().expect("Semantic Versioning carries translation parts");
    let embed = definitions(
        parts
            .embed
            .and_then(|part| part.source)
            .expect("Semantic Versioning carries an embedding"),
    );
    assert!(embed.contains(&"semver-embed"), "{embed:?}");
    assert!(embed.contains(&"semver-unembed"), "{embed:?}");
    for name in render.iter().chain(embed.iter()) {
        assert!(
            name.starts_with("semver-"),
            "{name} is not named for semver"
        );
    }
    for name in &embed {
        assert!(
            !render.contains(name),
            "{name} is defined by the render too"
        );
    }
}
