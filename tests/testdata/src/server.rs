// Rust: 4-space, braces, nested blocks, closures.

use std::collections::HashMap;

pub struct Server {
    routes: HashMap<String, Handler>,
}

type Handler = Box<dyn Fn(&str) -> String + Send + Sync>;

impl Server {
    pub fn new() -> Self {
        Server {
            routes: HashMap::new(),
        }
    }

    pub fn route<F>(&mut self, path: &str, handler: F)
    where
        F: Fn(&str) -> String + Send + Sync + 'static,
    {
        self.routes.insert(path.to_string(), Box::new(handler));
    }

    pub fn handle(&self, path: &str, input: &str) -> Option<String> {
        self.routes.get(path).map(|h| h(input))
    }
}

fn fibonacci(n: u64) -> u64 {
    if n < 2 {
        n
    } else {
        let mut a = 0;
        let mut b = 1;
        for _ in 0..n {
            let next = a + b;
            a = b;
            b = next;
        }
        a
    }
}
