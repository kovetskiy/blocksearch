<?php
// PHP: searching the method name returns the whole method body.
namespace Billing;

class InvoiceService
{
    public function computeTotal(float $subtotal): float
    {
        $tax = $subtotal * 0.20;
        return $subtotal + $tax;
    }
}
