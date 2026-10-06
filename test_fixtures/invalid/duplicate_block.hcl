resource "container" "base" {
  command = ["consul", "agent", "-dev", "-client", "0.0.0.0"]

  resources {
    cpu = 2000
  }

  resources {
    memory = 1024
  }
}
