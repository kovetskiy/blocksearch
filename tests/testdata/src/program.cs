// C#: a class with a method. Searching the method name returns the whole
// method body, braces included.
using System;

namespace Billing.Domain;

public class InvoiceCalculator
{
    private const decimal TaxRate = 0.20m;

    public decimal ComputeTotal(decimal subtotal)
    {
        var tax = subtotal * TaxRate;
        return subtotal + tax;
    }
}
