#!/bin/bash

echo "=== Kairon Template Synchronization Analysis ==="
echo

# Run comparison and capture output
comparison_output=$(go run scripts/compare-templates.go 2>/dev/null)
exit_code=$?

if [[ $exit_code -eq 0 ]]; then
    echo "✅ Templates are synchronized with live configurations"
    exit 0
fi

# Parse JSON output for summary
missing_count=$(echo "$comparison_output" | jq '.missing_in_templates | length')
diff_count=$(echo "$comparison_output" | jq '.content_differences | length') 
total_sync_needed=$(echo "$comparison_output" | jq '.summary.sync_needed')

echo "❌ Synchronization required: $total_sync_needed files need updates"
echo
echo "📊 Summary:"
echo "  • Files missing from templates: $missing_count"
echo "  • Files with content differences: $diff_count"
echo

if [[ $missing_count -gt 0 ]]; then
    echo "🆕 Files to add to templates:"
    echo "$comparison_output" | jq -r '.missing_in_templates[] | "   - " + .'
    echo
fi

if [[ $diff_count -gt 0 ]]; then
    echo "🔄 Files needing content sync:"
    echo "$comparison_output" | jq -r '.content_differences[] | "   - " + .path + " (live is newer)"'
    echo
fi

echo "💡 Next steps:"
echo "   1. Run Task 2-5 to synchronize all differences"
echo "   2. Verify with 'kairon init' in test directory" 
echo "   3. Ensure fixtures directory is properly created"

exit 1