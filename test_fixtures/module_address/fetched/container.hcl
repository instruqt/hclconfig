// stands in for what a module getter writes to disk
resource "container" "postgres" {
  command = ["postgres"]
}
