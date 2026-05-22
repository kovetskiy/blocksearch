# Elixir: searching the function name returns the whole def clause.
defmodule Billing.Worker do
  use GenServer

  def start_link(opts) do
    GenServer.start_link(__MODULE__, opts, name: __MODULE__)
  end

  def process_invoice(invoice) do
    {:ok, invoice}
  end
end
