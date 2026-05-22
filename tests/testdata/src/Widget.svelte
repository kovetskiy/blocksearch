<!--
  Svelte: searching the component name returns the whole element.
-->
<script>
  let total = 0;

  function add(amount) {
    total += amount;
  }
</script>

<BillingWidget label="Invoice">
  <p>Total: {total}</p>
</BillingWidget>
