// Canonical 2-space JavaScript.

const fs = require("fs");
const path = require("path");

function readConfig(file) {
  const raw = fs.readFileSync(file, "utf8");
  try {
    return JSON.parse(raw);
  } catch (err) {
    console.error("bad config", err);
    return {};
  }
}

class Server {
  constructor(port) {
    this.port = port;
    this.routes = {};
  }

  addRoute(path, handler) {
    if (typeof handler !== "function") {
      throw new Error("handler must be a function");
    }
    this.routes[path] = handler;
  }

  start() {
    console.log(`listening on ${this.port}`);
  }
}

module.exports = { readConfig, Server };
