Waybackrobots
=============

Returns disallowed paths from robots.txt found on your target domain and snapshotted by the Wayback Machine.

Inspired by [mhmdiaa/waybackrobots.py](https://gist.github.com/mhmdiaa/2742c5e147d49a804b408bfed3d32d07)

## Installation

```
go install github.com/vodafon/waybackrobots@latest
```

Or

```
go get -u github.com/vodafon/waybackrobots
```

## Usage

```
waybackrobots -d target.com
```

And you can use the `-raw` flag to print the robots files as-is.

```
waybackrobots -d target.com -raw
```

Filter by dates

```
waybackrobots -d target.com -start_date "2023-01-01"
waybackrobots -d target.com -end_date "2025-01-01"
waybackrobots -d target.com -start_date "2023-01-01" -end_date "2025-01-01"
```
