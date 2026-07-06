use std::fs;

use wasi_nn::{ExecutionTarget, GraphBuilder, GraphEncoding, TensorType};

fn main() {
    let xml = fs::read_to_string("fixture/model.xml").unwrap();
    let preview_len = xml.len().min(50);
    println!(
        "Read graph XML, first {} characters: {}",
        preview_len,
        &xml[..preview_len]
    );

    let weights = fs::read("fixture/model.bin").unwrap();
    println!("Read graph weights, size in bytes: {}", weights.len());

    let graph = GraphBuilder::new(GraphEncoding::Openvino, ExecutionTarget::CPU)
        .build_from_bytes(vec![xml.as_bytes(), &weights])
        .unwrap();
    println!("Loaded graph into wasi-nn");

    let mut ctx = graph.init_execution_context().unwrap();
    println!("Created wasi-nn execution context");

    let tensor_data = fs::read("fixture/tensor.bgr").unwrap();
    println!("Read input tensor, size in bytes: {}", tensor_data.len());

    let dimensions = vec![1, 3, 224, 224];
    ctx.set_input(0, TensorType::F32, &dimensions, &tensor_data)
        .unwrap();
    println!("Set input tensor");

    ctx.compute().unwrap();
    println!("Executed graph inference");

    let mut output_buffer = vec![0f32; 1001];
    ctx.get_output(0, &mut output_buffer).unwrap();
    println!(
        "Found results, sorted top 5: {:?}",
        &sort_results(&output_buffer)[..5]
    );
}

fn sort_results(buffer: &[f32]) -> Vec<InferenceResult> {
    let mut results: Vec<InferenceResult> = buffer
        .iter()
        .skip(1)
        .enumerate()
        .map(|(class, probability)| InferenceResult(class, *probability))
        .collect();
    results.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap());
    results
}

#[derive(Debug, PartialEq)]
struct InferenceResult(usize, f32);
